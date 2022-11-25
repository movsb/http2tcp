package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	server    string
	token     string
	userAgent string
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

// If non-empty, when connecting to the server, this User-Agent will be used
// instead of the default `Go-http-client/1.1`.
func (c *Client) SetUserAgent(userAgent string) {
	c.userAgent = userAgent
}

func (c *Client) Std(to string) {
	std := NewStdReadWriteCloser()
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
			log.Println(err)
			time.Sleep(time.Second * 5)
			continue
		}
		go func(conn io.ReadWriteCloser) {
			if err := c.proxy(conn, to); err != nil {
				log.Println(err)
			}
		}(conn)
	}
}

func (c *Client) proxy(local io.ReadWriteCloser, addr string) error {
	onceCloseLocal := &OnceCloser{Closer: local}
	defer onceCloseLocal.Close()

	remote, bior, err := c.dialServer(addr)
	if err != nil {
		return err
	}
	onceCloseRemote := &OnceCloser{Closer: remote}
	defer onceCloseRemote.Close()

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()

		defer onceCloseRemote.Close()
		_, _ = io.Copy(remote, local)
	}()

	go func() {
		defer wg.Done()

		if n := int64(bior.Buffered()); n > 0 {
			if nc, err := io.CopyN(local, bior, n); err != nil || nc != n {
				log.Println("io.CopyN:", nc, err)
				return
			}
		}

		defer onceCloseLocal.Close()
		_, _ = io.Copy(local, remote)
	}()

	wg.Wait()
	return nil
}

func (c *Client) dialServer(destination string) (net.Conn, *bufio.Reader, error) {
	u, err := url.Parse(c.server)
	if err != nil {
		return nil, nil, err
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
			return nil, nil, fmt.Errorf(`unknown scheme: %s`, u.Scheme)
		}
	}
	serverAddr := net.JoinHostPort(host, port)

	var remote net.Conn
	if u.Scheme == `http` {
		remote, err = net.Dial(`tcp`, serverAddr)
		if err != nil {
			return nil, nil, err
		}
	} else if u.Scheme == `https` {
		remote, err = tls.Dial(`tcp`, serverAddr, nil)
		if err != nil {
			return nil, nil, err
		}
	}
	if remote == nil {
		return nil, nil, fmt.Errorf("no server connection made")
	}

	v := u.Query()
	v.Set(`addr`, destination)
	body := strings.NewReader(v.Encode())

	req, err := http.NewRequest(http.MethodPost, u.String(), body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Add(`Connection`, `upgrade`)
	req.Header.Add(`Upgrade`, httpHeaderUpgrade)
	req.Header.Add(`Authorization`, fmt.Sprintf(`%s %s`, authHeaderType, c.token))
	req.Header.Add(`Content-Type`, `application/x-www-form-urlencoded`)
	req.Header.Add(`Content-Length`, fmt.Sprint(body.Len()))
	if c.userAgent != `` {
		req.Header.Add(`User-Agent`, c.userAgent)
	}

	if err := req.Write(remote); err != nil {
		return nil, nil, err
	}
	bior := bufio.NewReader(remote)
	resp, err := http.ReadResponse(bior, req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		buf := bytes.NewBuffer(nil)
		resp.Write(buf)
		return nil, nil, fmt.Errorf("statusCode != 101:\n%s", buf.String())
	}
	return remote, bior, nil
}
