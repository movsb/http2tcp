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
	"reflect"
	"strings"
	"sync"
	"time"
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
	defer func() {
		if err := onceCloseLocal.Close(); err != nil {
			log.Println("error closing local", err)
			return
		}
	}()

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

	var remote net.Conn
	if u.Scheme == `http` {
		remote, err = net.Dial(`tcp`, serverAddr)
		if err != nil {
			return err
		}
	} else if u.Scheme == `https` {
		remote, err = tls.Dial(`tcp`, serverAddr, nil)
		if err != nil {
			return err
		}
	}
	if remote == nil {
		return fmt.Errorf("no server connection made")
	}

	onceCloseRemote := &OnceCloser{Closer: remote}
	defer func() {
		if err := onceCloseRemote.Close(); err != nil {
			log.Println("error closing server connection", err)
			return
		}
	}()

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
	if err := req.Write(remote); err != nil {
		return err
	}
	bior := bufio.NewReader(remote)
	resp, err := http.ReadResponse(bior, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		buf := bytes.NewBuffer(nil)
		resp.Write(buf)
		return fmt.Errorf("statusCode != 101:\n%s", buf.String())
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()

		if _, err := io.Copy(remote, local); err != nil {
			err2 := onceCloseRemote.Close()
			log.Println("close remote because local is abnormally closed", err, err2)
			return
		}

		if cw, ok := remote.(WriteCloser); ok {
			err := cw.CloseWrite()
			log.Println("close write of remote because local read is normally closed", err)
			return
		}

		err = onceCloseRemote.Close()
		log.Println("close remote because local is normally closed but remote is not a WriteCloser", reflect.TypeOf(remote).String(), err)
	}()

	go func() {
		defer wg.Done()

		if n := int64(bior.Buffered()); n > 0 {
			if nc, err := io.CopyN(local, bior, n); err != nil || nc != n {
				log.Println("io.CopyN:", nc, err)
				return
			}
		}

		if _, err := io.Copy(local, remote); err != nil {
			err2 := onceCloseLocal.Close()
			log.Println("close local because remote is abnormally closed", err, err2)
			return
		}

		if cw, ok := local.(WriteCloser); ok {
			err := cw.CloseWrite()
			log.Println("close write of local because remote is normally closed", err)
			return
		}

		err = onceCloseLocal.Close()
		log.Println("close local because remote is normally closed but local is not a WriteCloser", reflect.TypeOf(local).String(), err)
	}()

	wg.Wait()
	return nil
}
