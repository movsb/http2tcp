package main

import (
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
)

const (
	httpHeaderUpgrade = `http2tcp/1.0`
)

type Server struct {
	privateKey PrivateKey
	conn       int32 // number of active connections
}

func NewServer(privateKey PrivateKey) *Server {
	return &Server{
		privateKey: privateKey,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if upgrade := r.Header.Get(`Upgrade`); upgrade != httpHeaderUpgrade {
		http.Error(w, `upgrade error`, http.StatusBadRequest)
		return
	}

	if r.ContentLength < 32 || r.ContentLength > 1<<10 {
		http.Error(w, `invalid address`, http.StatusBadRequest)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `invalid body`, http.StatusBadRequest)
		return
	}
	publicKey := PublicKey{}
	copy(publicKey[:], body[:32])

	g, err := NewAesGcm(s.privateKey.SharedSecret(publicKey))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	destination, err := g.Decrypt(body[32:])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// the URL.Path doesn't matter.
	remote, err := net.Dial(`tcp`, string(destination))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	onceCloseRemote := &OnceCloser{Closer: remote}
	defer onceCloseRemote.Close()

	w.Header().Add(`Content-Length`, `0`)
	w.WriteHeader(http.StatusSwitchingProtocols)
	local, bio, err := w.(http.Hijacker).Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	onceCloseLocal := &OnceCloser{Closer: local}
	defer onceCloseLocal.Close()

	log.Println("enter: number of connections:", atomic.AddInt32(&s.conn, +1))
	defer func() { log.Println("leave: number of connections:", atomic.AddInt32(&s.conn, -1)) }()

	log.Println(`User:`, publicKey.String(), `Destination:`, string(destination))

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()

		// The returned bufio.Reader may contain unprocessed buffered data from the client.
		// Copy them to dst so we can use src directly.
		if n := bio.Reader.Buffered(); n > 0 {
			n64, err := io.CopyN(remote, bio, int64(n))
			if n64 != int64(n) || err != nil {
				log.Println("io.CopyN:", n64, err)
				return
			}
		}

		defer onceCloseRemote.Close()
		_, _ = io.Copy(remote, local)
	}()

	go func() {
		defer wg.Done()

		// flush any unwritten data.
		if err := bio.Writer.Flush(); err != nil {
			log.Println(`bio.Writer.Flush():`, err)
			return
		}

		defer onceCloseLocal.Close()
		_, _ = io.Copy(local, remote)
	}()

	wg.Wait()
}
