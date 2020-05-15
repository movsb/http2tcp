package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
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
					httpAddress := config.Server
					if !strings.Contains(httpAddress, `://`) {
						httpAddress = `http://` + httpAddress
					}
					u, err := url.Parse(httpAddress)
					if err != nil {
						panic(err)
					}
					host := u.Hostname()
					port := u.Port()
					if port == `` {
						switch u.Scheme {
						case `http`:
							port = "80"
						case `https`:
							port = `443`
						default:
							panic(`unknown scheme`)
						}
					}
					server, err := tls.Dial(`tcp`, fmt.Sprintf(`%s:%s`, host, port), nil)
					if err != nil {
						log.Println(err)
						return
					}
					defer server.Close()
					u.Path = fmt.Sprintf(`/%s/%s`, config.Prefix, path.Name)
					req, err := http.NewRequest(http.MethodGet, u.String(), nil)
					if err != nil {
						panic(err)
					}
					req.Header.Add(`Connection`, `upgrade`)
					req.Header.Add(`Upgrade`, `http2tcp`)
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
