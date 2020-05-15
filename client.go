package main

import (
	"bufio"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
)

func client() {
	wg := sync.WaitGroup{}
	for _, path := range config.Paths {
		wg.Add(1)
		go func(path Path) {
			defer wg.Done()
			listener, err := net.Listen(`tcp`, path.Local)
			if err != nil {
				panic(err)
			}
			defer listener.Close()
			for {
				conn, err := listener.Accept()
				if err != nil {
					panic(err)
				}
				log.Println(`accept:`, conn.RemoteAddr().String())
				go func() {
					defer conn.Close()
					server, err := tls.Dial(`tcp`, `blog.twofei.com:443`, nil)
					if err != nil {
						panic(err)
					}
					defer server.Close()
					req, err := http.NewRequest(http.MethodGet, `https://blog.twofei.com/`+config.Prefix+`/`+path.Name, nil)
					if err != nil {
						panic(err)
					}
					req.Header.Add(`Connection`, `upgrade`)
					req.Header.Add(`Upgrade`, `nginx2tcp`)
					req.Header.Add(`Authorization`, path.Token)
					if err := req.Write(server); err != nil {
						panic(err)
					}
					bior := bufio.NewReader(server)
					resp, err := http.ReadResponse(bior, req)
					if err != nil {
						panic(err)
					}
					defer resp.Body.Close()
					if resp.StatusCode != 101 {
						log.Println(`status !=101`)
						return
					}
					wg := &sync.WaitGroup{}
					wg.Add(2)

					go func() {
						defer wg.Done()
						io.Copy(conn, bior)
					}()

					go func() {
						defer wg.Done()
						io.Copy(server, conn)
					}()

					wg.Wait()
				}()
			}
		}(path)
	}
	wg.Wait()
}
