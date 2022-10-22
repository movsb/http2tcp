package main

import (
	"io"
	"os"
	"sync"
)

type WriteCloser interface {
	CloseWrite() error
}

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
	io.ReadCloser
	io.WriteCloser

	closed      bool
	writeClosed bool
	mu          sync.Mutex
}

func NewStdReadWriteCloser() *StdReadWriteCloser {
	return &StdReadWriteCloser{
		ReadCloser:  os.Stdin,
		WriteCloser: os.Stdout,
	}
}

func (c *StdReadWriteCloser) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return os.ErrInvalid
	}

	c.closed = true

	var (
		err1 error
		err2 error
	)

	err1 = c.ReadCloser.Close()
	if !c.writeClosed {
		err2 = c.WriteCloser.Close()
		c.writeClosed = true
	}

	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}

	return nil
}

func (c *StdReadWriteCloser) CloseWrite() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.writeClosed {
		return os.ErrInvalid
	}
	c.writeClosed = true
	return c.WriteCloser.Close()
}
