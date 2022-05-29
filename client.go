package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

type Client struct {
	server string
	token  string
}

func NewClient(server string, token string) *Client {
	if !strings.Contains(server, "://") {
		server = "http://" + server
	}
	return &Client{
		server: server,
		token:  token,
	}
}

type _stdReadWriter struct {
	io.Reader
	io.Writer
}

func (c *Client) Std(to string) {
	std := _stdReadWriter{os.Stdin, os.Stdout}
	if err := c.proxy(std, to); err != nil {
		log.Println(err)
	}
}

func (c *Client) Serve(listen string, to string) {
	lis, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatalln(err)
	}
	defer lis.Close()

	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Fatalln(err)
		}
		go func(conn io.ReadWriteCloser) {
			defer conn.Close()
			if err := c.proxy(conn, to); err != nil {
				log.Println(err)
			}
		}(conn)
	}
}

func (c *Client) proxy(clientConn io.ReadWriter, addr string) error {
	u, err := url.Parse(c.server)
	if err != nil {
		return err
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
			return fmt.Errorf(`unknown scheme: %s`, u.Scheme)
		}
	}
	serverAddr := net.JoinHostPort(host, port)

	var serverConn net.Conn
	if u.Scheme == `http` {
		serverConn, err = net.Dial(`tcp`, serverAddr)
		if err != nil {
			return err
		}
	} else if u.Scheme == `https` {
		serverConn, err = tls.Dial(`tcp`, serverAddr, nil)
		if err != nil {
			return err
		}
	}
	if serverConn == nil {
		return fmt.Errorf("no server connection made")
	}

	defer serverConn.Close()

	v := u.Query()
	v.Set(`addr`, addr)
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Add(`Connection`, `upgrade`)
	req.Header.Add(`Upgrade`, httpHeaderUpgrade)
	req.Header.Add(`Authorization`, fmt.Sprintf(`%s %s`, authHeaderType, c.token))
	if err := req.Write(serverConn); err != nil {
		return err
	}
	bior := bufio.NewReader(serverConn)
	resp, err := http.ReadResponse(bior, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		b, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("statusCode != 101: %s: %s", resp.Status, string(b))
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()

		if n := int64(bior.Buffered()); n > 0 {
			if nc, err := io.CopyN(clientConn, bior, n); err != nil || nc != n {
				return
			}
		}

		io.Copy(clientConn, serverConn)
	}()

	go func() {
		defer wg.Done()

		io.Copy(serverConn, clientConn)
	}()

	wg.Wait()
	return nil
}
