package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
)

func server() {
	handlerOf := func(path Path) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			log.Println(req.RequestURI)
			authorization := req.Header.Get(`Authorization`)
			upgrade := req.Header.Get(`Upgrade`)
			if authorization != path.Token || upgrade != `http2tcp` {
				log.Println(`unauth`)
				return
			}
			w.Header().Add(`Content-Length`, `0`)
			w.WriteHeader(http.StatusSwitchingProtocols)
			remote, err := net.Dial(`tcp`, path.Remote)
			if err != nil {
				panic(err)
			}
			defer remote.Close()

			conn, bio, _ := w.(http.Hijacker).Hijack()
			defer conn.Close()

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
				io.Copy(remote, conn)
			}()

			go func() {
				defer wg.Done()
				io.Copy(conn, remote)
			}()

			wg.Wait()
		}
	}
	mux := http.NewServeMux()
	for _, path := range config.Paths {
		u := fmt.Sprintf(`/%s/%s`, config.Prefix, path.Name)
		mux.HandleFunc(
			u,
			handlerOf(path),
		)
		log.Println(`handle:`, u)
	}
	http.ListenAndServe(config.Listen, mux)
}
