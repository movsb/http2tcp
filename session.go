package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"
)

// 一个连接相关的会话信息。
// 因为本工具上层承载的是 TCP 连接，所以本质是五个传输层。
// 所以为了不把传输层的错误抛给上层，特地处理会话信息。
// 主要是处理客户端与服务端连接断开后的连接恢复。
// 以此表现得像五个永远不断开的连接。
type Session struct {
	ctx context.Context

	// 从 1 开始
	id int64

	// 对于本地来说，是本地连接。
	// 对于远程来说，是远程连接。
	conn io.ReadWriteCloser

	// 接收包序列号（从 1 开始）
	txSeq int64

	// 发送包序列号（从 1 开始）
	rxSeq int64

	// TODO：读写分离
	transporter       *Transporter
	lockTransporter   sync.RWMutex
	chWaitTransporter chan *Transporter

	// 客户端用的
	isClient bool
	server   string
	token    string
	connect  string
}

func NewClientSession(conn io.ReadWriteCloser, server string, token string, connect string) *Session {
	return &Session{
		id:       0,
		txSeq:    1,
		rxSeq:    1,
		conn:     conn,
		server:   server,
		token:    token,
		connect:  connect,
		isClient: true,
	}
}

func NewServerSession(id int64, conn io.ReadWriteCloser, transporter *Transporter) *Session {
	return &Session{
		id:                id,
		conn:              conn,
		txSeq:             1,
		rxSeq:             1,
		transporter:       transporter,
		isClient:          false,
		chWaitTransporter: make(chan *Transporter),
	}
}

// TODO：将来可以绑定多个，以实现读写分离
func (s *Session) BindTransporter(transporter *Transporter) {
	s.chWaitTransporter <- transporter
}

func (s *Session) Run(ctx context.Context) error {
	s.ctx = ctx

	defer log.Println(`session ended`)

	defer func() {
		if err := s.conn.Close(); err != nil {
			log.Println(`error closing session conn:`, err)
		}
	}()

	if s.isClient {
		t, sid, err := NewClientTransporter(ctx, s.server, s.token, s.connect, 0)
		if err != nil {
			return fmt.Errorf(`failed to create first session transport: %w`, err)
		}
		s.id = sid
		s.transporter = t
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer log.Println(`loopWrites exited`)
		s.loopWrites(ctx)
	}()
	go func() {
		defer wg.Done()
		defer log.Println(`loopReads exited`)
		s.loopReads(ctx)
	}()

	wg.Wait()

	return nil
}

func (s *Session) resetTransporter(old *Transporter) error {
	s.lockTransporter.Lock()
	defer s.lockTransporter.Unlock()

	// 关两次没效果
	old.Close()
	old.Close()

	if old != s.transporter {
		log.Println(`already reset, use instead`)
		return nil
	}

	if s.isClient {
		t, sid, err := NewClientTransporter(s.ctx, s.server, s.token, s.connect, s.id)
		if err != nil {
			return fmt.Errorf(`error resetting transport: %w`, err)
		}
		if sid != s.id {
			panic(`sid != s.id`)
		}

		log.Println(`transporter reset`)

		s.transporter = t
		return nil
	}

	// 要等待主动连接，但是最多等10分钟
	// TODO
	select {
	case s.transporter = <-s.chWaitTransporter:
		log.Println(`client reconnected`)
		return nil
	case <-time.After(time.Minute * 5):
		log.Println(`timed out, client not reconnected`)
		return fmt.Errorf(`actively closed: %v`, `timed out`)
	}
}

func (s *Session) getTransporter() *Transporter {
	s.lockTransporter.RLock()
	t := s.transporter
	s.lockTransporter.RUnlock()
	return t
}

// 读远程，写本地。
func (s *Session) loopReads(ctx context.Context) {
	for {
		reset := false
		data := RelayData{}

		transporter := s.getTransporter()

		for {
			if reset {
				if err := s.resetTransporter(transporter); err != nil {
					if strings.Contains(err.Error(), `actively closed`) {
						return
					}
					log.Println(`failed to reset transporter, trying again:`, err)
					time.Sleep(time.Second * 3)
					continue
				}
				transporter = s.getTransporter()
			}

			if err := transporter.Read(&data); err != nil {
				log.Println(`error decoding data, trying again:`, err)
				reset = true
				continue
			}

			break
		}

		// 数据校验
		switch {
		case data.Seq < s.rxSeq:
			log.Println(`redundant data received, dropping:`, data.Seq)
			continue
		case data.Seq > s.rxSeq:
			log.Println(`invalid data received, exiting:`, data.Seq, s.rxSeq)
			return
		}

		if nw, err := s.conn.Write(data.Data); nw != len(data.Data) || err != nil {
			log.Println(`failed to write local/remote, exiting:`, nw, err)
			return
		}

		// 读取成功
		log.Printf(`recv seq: %v, bytes: %d`, s.rxSeq, len(data.Data))
		s.rxSeq++
	}
}

// 读本地，写远程。
func (s *Session) loopWrites(ctx context.Context) {
	buf := make([]byte, 16<<10)

	for {
		nr, err := s.conn.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			log.Println(`failed to read local/remote, exiting:`, err)
			return
		}

		data := RelayData{
			Seq:  s.txSeq,
			Data: buf[:nr],
		}

		reset := false
		transporter := s.getTransporter()

		for {
			if reset {
				if err := s.resetTransporter(transporter); err != nil {
					if strings.Contains(err.Error(), `actively closed`) {
						return
					}
					log.Println(`failed to reset transporter, trying again:`, err)
					time.Sleep(time.Second * 3)
					continue
				}
				transporter = s.getTransporter()
			}

			if err := transporter.Write(&data); err != nil {
				log.Println(`error write data, trying again:`, err)
				reset = true
				continue
			}

			break
		}

		// 发送成功
		log.Printf(`sent seq: %v, bytes: %d`, s.txSeq, len(data.Data))
		s.txSeq++
	}
}
