package main

import (
	"io"
	"log"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/movsb/http2tcp"
)

type Client struct {
	// https://host/path/?query
	Server    string
	Token     string
	UserAgent string
}

// If non-empty, when connecting to the server, this User-Agent will be used
// instead of the default `Go-http-client/1.1`.
func (c *Client) SetUserAgent(userAgent string) {
	c.UserAgent = userAgent
}

func (c *Client) Std(to string) {
	std := NewStdReadWriteCloser()
	c.Conn(std, to)
}

func (c *Client) Conn(conn io.ReadWriteCloser, to string) {
	if err := c.proxy(conn, to); err != nil {
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
		go c.Conn(conn, to)
	}
}

func (c *Client) proxy(local io.ReadWriteCloser, addr string) error {
	onceCloseLocal := &OnceCloser{Closer: local}
	defer onceCloseLocal.Close()

	u, err := url.Parse(c.Server)
	if err != nil {
		log.Println(err)
		return err
	}

	a := u.Query()
	a.Set(`addr`, addr)
	u.RawQuery = a.Encode()

	remote, err := http2tcp.Dial(u.String(), c.Token, c.UserAgent)
	if err != nil {
		log.Println(err)
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

		defer onceCloseLocal.Close()
		_, _ = io.Copy(local, remote)
	}()

	wg.Wait()
	return nil
}
