package main

import (
	"context"
	"io"
	"log"
	"net"
	"strings"
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

	session := NewClientSession(local, c.server, c.token, addr)
	return session.Run(context.Background())
}
