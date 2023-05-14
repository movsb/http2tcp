package main

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
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

	// 发送包序列号（从 1 开始）
	// 下一个包的。
	// 发送成功后加一
	txSeq atomic.Int64
	// txCnt       atomic.Int64
	txQueue     *list.List
	txQueueLock sync.Mutex

	// 接收包序列号（从 1 开始）
	// 希望接收到的。
	// 接收成功后加一。
	rxSeq atomic.Int64
	// rxCnt atomic.Int64

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
	s := &Session{
		id:       0,
		conn:     conn,
		server:   server,
		token:    token,
		connect:  connect,
		isClient: true,
	}
	s.txSeq.Store(1)
	s.rxSeq.Store(1)
	s.txQueue = list.New()
	return s
}

func NewServerSession(id int64, conn io.ReadWriteCloser, transporter *Transporter) *Session {
	s := &Session{
		id:                id,
		conn:              conn,
		transporter:       transporter,
		isClient:          false,
		chWaitTransporter: make(chan *Transporter),
	}
	s.txSeq.Store(1)
	s.rxSeq.Store(1)
	s.txQueue = list.New()
	return s
}

// TODO：将来可以绑定多个，以实现读写分离
func (s *Session) BindTransporter(transporter *Transporter) {
	log.Println(`enter bind`)
	defer log.Println(`leave bind`)
	s.lockTransporter.Lock()
	s.transporter.Close()
	s.lockTransporter.Unlock()
	s.chWaitTransporter <- transporter
}

func (s *Session) Run(ctx context.Context) error {
	s.ctx = ctx

	defer log.Println(`session ended`)

	onceClose := OnceCloser{Closer: s.conn}
	defer onceClose.Close()

	defer func() {
		// 如果 reset 失败（比如服务器主动关闭，这是可以为空的。
		if t := s.getTransporter(); t != nil {
			t.Close()
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
		defer onceClose.Close()
		defer log.Println(`loopWrites exited`)
		s.loopWrites(ctx)
	}()
	go func() {
		defer wg.Done()
		defer onceClose.Close()
		defer log.Println(`loopReads exited`)
		s.loopReads(ctx)
	}()

	wg.Wait()

	return nil
}

func (s *Session) resetTransporter(old *Transporter) error {
	s.lockTransporter.Lock()

	// 关两次没效果
	old.Close()

	if old != s.transporter {
		s.lockTransporter.Unlock()
		log.Println(`already reset, use instead`)
		return nil
	}

	if s.isClient {
		defer s.lockTransporter.Unlock()

		t, sid, err := NewClientTransporter(s.ctx, s.server, s.token, s.connect, s.id)
		if err != nil {
			s.transporter = nil
			return fmt.Errorf(`error resetting transport: %w`, err)
		}
		if sid != s.id {
			panic(`sid != s.id`)
		}

		log.Println(`transporter reset`)

		s.transporter = t
		return nil
	}

	s.lockTransporter.Unlock()

	// 要等待主动连接，但是最多等10分钟
	// TODO
	log.Println(`waiting for client reconnection`)
	defer log.Println(`waited for client reconnection`)
	select {
	case s.transporter = <-s.chWaitTransporter:
		log.Println(`client reconnected`)
		return nil
	case <-time.After(time.Minute):
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
	reset := false
	transporter := s.getTransporter()

	for {
		data := RelayData{}

		// if s.isClient && s.txCnt >= 1<<20 {
		// 	log.Println(`data reached limit, resetting`)
		// 	reset = true
		// }

		for {
			if reset {
				if err := s.resetTransporter(transporter); err != nil {
					if strings.Contains(err.Error(), `actively closed`) {
						log.Println(`server closed, exiting`)
						return
					}
					log.Println(`failed to reset transporter, trying again:`, err)
					time.Sleep(time.Second * 3)
					continue
				}
				reset = false
			}

			transporter = s.getTransporter()

			if err := transporter.Read(&data); err != nil {
				log.Println(`error decoding data, trying again:`, err)
				reset = true
				continue
			}

			break
		}

		// 数据校验
		switch {
		case data.TxSeq < s.rxSeq.Load():
			log.Printf(`redundant data received, dropping: data=%d, session=%d`, data.TxSeq, s.rxSeq.Load())
			continue
		case data.TxSeq > s.rxSeq.Load():
			log.Printf(`invalid data received, exiting: data=%d, session=%d`, data.TxSeq, s.rxSeq.Load())
			transporter.Close()
			return
		}

		if nw, err := s.conn.Write(data.Data); nw != len(data.Data) || err != nil {
			log.Println(`failed to write local/remote, exiting:`, nw, err)
			return
		}

		// 读取成功
		log.Printf(`recv seq: %v, bytes: %d, tid: %d`, s.rxSeq.Load(), len(data.Data), transporter.GetID())
		s.rxSeq.Add(1)

		// 发送太久，重置。
		if time.Since(data.Time) > time.Second*5 {
			log.Println(`timed out waiting for data, resetting`)
			reset = true
		}

		// s.rxCnt.Add(int64(len(data.Data)))

		// 对方成功读取数据后删除内存数据
		nRemoved := 0
		s.txQueueLock.Lock()
		for {
			front := s.txQueue.Front()
			if front == nil {
				break // should be here
			}
			elem := front.Value.(*RelayData)
			if elem.TxSeq < data.RxSeq {
				s.txQueue.Remove(front)
				nRemoved++
				continue
			}
			break
		}
		s.txQueueLock.Unlock()
		log.Printf(`removed %d data in tx queue`, nRemoved)
	}
}

// 读本地，写远程。
func (s *Session) loopWrites(ctx context.Context) {
	buf := make([]byte, 16<<10)
	reset := false

	for {
		nr, err := s.conn.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Println(`net.conn eof, exiting`)
				return
			}
			log.Println(`failed to read local/remote, exiting:`, err)
			return
		}

		data := RelayData{
			TxSeq: s.txSeq.Load(),
			RxSeq: s.rxSeq.Load(),
			Data:  buf[:nr],
			Time:  time.Now(),
		}
		s.txSeq.Add(1)

		// 写入队列，发送，并等待对方成功读取后再删除
		s.txQueueLock.Lock()
		s.txQueue.PushBack(&data)
		if s.txQueue.Len() > 100 {
			log.Println(`too many data in tx queue:`, s.txQueue.Len())
		}
		s.txQueueLock.Unlock()

		transporter := s.getTransporter()
		if transporter == nil {
			log.Println(`transporter closed, exiting`)
			return
		}

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
				reset = false
				goto reconnected
			}

			transporter = s.getTransporter()
			if transporter == nil {
				log.Println(`transporter closed, exiting`)
				return
			}

			if err := transporter.Write(data); err != nil {
				log.Println(`error write data, trying again:`, err)
				reset = true
				continue
			}

			// 发送成功
			log.Printf(`sent seq: %v, bytes: %d, tid: %d`, data.TxSeq, len(data.Data), transporter.GetID())
			goto next

		}

	reconnected:
		for front := (*list.Element)(nil); ; {
			s.txQueueLock.Lock()
			if front == nil {
				front = s.txQueue.Front()
			} else {
				front = front.Next()
			}
			s.txQueueLock.Unlock()
			if front == nil {
				break // should be here
			}

			data := front.Value.(*RelayData)

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
					reset = false
				}

				transporter = s.getTransporter()
				if transporter == nil {
					log.Println(`transporter closed, exiting`)
					return
				}

				if err := transporter.Write(data); err != nil {
					log.Println(`error write data, trying again:`, err)
					reset = true
					continue
				}

				// 发送成功
				log.Printf(`sent seq: %v, bytes: %d, tid: %d`, data.TxSeq, len(data.Data), transporter.GetID())

				break
			}
		}
	next:
	}
}
