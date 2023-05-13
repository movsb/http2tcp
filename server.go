package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"nhooyr.io/websocket"
)

const (
	authHeaderType    = `HTTP2TCP`
	httpHeaderUpgrade = `http2tcp/1.0`
)

type Server struct {
	token string
	conn  int32 // number of active connections

	sessions          map[int64]*Session
	nextSessionID     int64
	lockNextSessionID sync.Mutex
	sessionsLock      sync.Mutex
}

func NewServer(token string) *Server {
	return &Server{
		token:         token,
		sessions:      make(map[int64]*Session),
		nextSessionID: 1,
	}
}

func (s *Server) auth(r *http.Request) bool {
	a := strings.Fields(r.Header.Get("Authorization"))
	if len(a) == 2 && a[0] == authHeaderType && a[1] == s.token {
		return true
	}
	return false
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.auth(r) {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	session /* may be nil */, err := s.acceptSession(w, r)
	if err != nil {
		log.Println(`failed to accept session:`, err)
		// already hijacked
		// http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	log.Println("enter: number of connections:", atomic.AddInt32(&s.conn, +1))
	defer func() { log.Println("leave: number of connections:", atomic.AddInt32(&s.conn, -1)) }()

	if session != nil {
		if err := session.Run(context.Background()); err != nil {
			log.Println(`failed to run session:`, err)
			return
		}
	}
}

func (s *Server) getNextSessionID() int64 {
	s.lockNextSessionID.Lock()
	defer s.lockNextSessionID.Unlock()
	id := s.nextSessionID
	s.nextSessionID++
	return id
}

// 注意，已经存在的会话不会返回。
func (s *Server) acceptSession(w http.ResponseWriter, r *http.Request) (*Session, error) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println(`failed to accept websocket session:`, err)
		return nil, err
	}

	shouldCloseConn := true
	defer func() {
		if shouldCloseConn {
			conn.Close(websocket.StatusInternalError, "closed in defer")
		}
	}()

	transporter := NewServerTransporter(context.Background(), conn)
	beginSessionRequest := BeginSessionRequest{}
	if err := transporter.Read(&beginSessionRequest); err != nil {
		log.Println(`failed to read begin session request:`, err)
		return nil, err
	}

	// 全新的连接
	if beginSessionRequest.SessionID == 0 {
		remote, err := net.Dial(`tcp`, beginSessionRequest.Connect)
		if err != nil {
			log.Println(`failed to dial remote:`, err)
			beginSessionResponse := BeginSessionResponse{
				SessionID: 0,
				Reason:    fmt.Sprintf(`failed to dial remote: %v`, err),
			}
			if err := transporter.Write(&beginSessionResponse); err != nil {
				return nil, fmt.Errorf(`failed to write begin session response: %w`, err)
			}
			return nil, err
		}

		sessionID := s.getNextSessionID()

		beginSessionResponse := BeginSessionResponse{
			SessionID: sessionID,
			Reason:    ``,
		}
		if err := transporter.Write(&beginSessionResponse); err != nil {
			return nil, fmt.Errorf(`failed to write begin session response: %w`, err)
		}

		session := NewServerSession(sessionID, remote, transporter)

		s.sessionsLock.Lock()
		s.sessions[sessionID] = session
		s.sessionsLock.Unlock()

		shouldCloseConn = false
		return session, nil
	}

	// 尝试恢复会话
	s.sessionsLock.Lock()

	session, ok := s.sessions[beginSessionRequest.SessionID]

	if !ok {
		s.sessionsLock.Unlock()

		log.Println(`session not found:`, beginSessionRequest.SessionID)
		beginSessionResponse := BeginSessionResponse{
			SessionID: 0,
			Reason:    fmt.Sprintf(`session not found: %v`, err),
		}
		if err := transporter.Write(&beginSessionResponse); err != nil {
			return nil, fmt.Errorf(`failed to write begin session response: %w`, err)
		}

		shouldCloseConn = false
		return session, nil
	}

	s.sessionsLock.Unlock()

	// 存在会话，更新到新的客户端
	session.BindTransporter(transporter)

	shouldCloseConn = false
	return nil, nil
}
