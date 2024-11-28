package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/movsb/http2tcp"
)

type Server struct {
	Token string
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, req, err := http2tcp.Accept(w, r, s.Token)
	if err != nil {
		log.Println(err)
		return
	}
	s.serve(conn, req)
}

func (s *Server) serve(conn io.ReadWriteCloser, req *http.Request) {
	onceCloseLocal := &OnceCloser{Closer: conn}
	defer onceCloseLocal.Close()

	// the URL.Path doesn't matter.
	addr := req.URL.Query().Get("addr")
	remote, err := net.Dial(`tcp`, addr)
	if err != nil {
		log.Println(err)
		return
	}
	onceCloseRemote := &OnceCloser{Closer: remote}
	defer onceCloseRemote.Close()

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()

		defer onceCloseRemote.Close()
		_, _ = io.Copy(remote, conn)
	}()

	go func() {
		defer wg.Done()

		defer onceCloseLocal.Close()
		_, _ = io.Copy(conn, remote)
	}()

	wg.Wait()
}
