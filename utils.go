package main

import (
	"io"
	"os"
	"sync"
)

type OnceCloser struct {
	io.Closer
	once sync.Once
}

func (c *OnceCloser) Close() (err error) {
	c.once.Do(func() {
		err = c.Closer.Close()
	})
	return
}

type StdReadWriteCloser struct {
	closed chan struct{}
	read   chan []byte
	err    error
}

func NewStdReadWriteCloser() *StdReadWriteCloser {
	std := &StdReadWriteCloser{
		closed: make(chan struct{}),
		read:   make(chan []byte),
	}
	go std.bgRead()
	return std
}

func (c *StdReadWriteCloser) bgRead() {
	for {
		var buf []byte
		select {
		case <-c.closed:
			return
		case buf = <-c.read:
		}
		n, err := os.Stdin.Read(buf)
		c.err = err
		c.read <- buf[:n]
	}
}

func (c *StdReadWriteCloser) Read(p []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, io.EOF
	case c.read <- p:
	}
	select {
	case <-c.closed:
		return 0, io.EOF
	case p = <-c.read:
		return len(p), c.err
	}
}

func (c *StdReadWriteCloser) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

func (c *StdReadWriteCloser) Close() error {
	close(c.closed)
	return nil
}
