package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	authHeaderType    = `HTTP2TCP`
	httpHeaderUpgrade = `http2tcp/1.0`
)

type Server struct {
	token string
	conn  int32 // number of active connections
}

func NewServer(token string) *Server {
	return &Server{
		token: token,
	}
}

func (s *Server) auth(r *http.Request) bool {
	a := strings.Fields(r.Header.Get("Authorization"))
	if len(a) == 2 && a[0] == authHeaderType && a[1] == s.token {
		return true
	}
	return false
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.auth(r) {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if upgrade := r.Header.Get(`Upgrade`); upgrade != httpHeaderUpgrade {
		http.Error(w, `upgrade error`, http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// the URL.Path doesn't matter.
	addr := r.PostFormValue(`addr`)
	remote, err := net.Dial(`tcp`, addr)
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
