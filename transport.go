package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

type Transporter struct {
	ctx context.Context

	conn *websocket.Conn

	isClient bool
	id       int64
}

type BeginSessionRequest struct {
	// 继续旧会话还是开始新会话
	SessionID int64

	// 被连接的地址
	Connect string
}

type BeginSessionResponse struct {
	SessionID int64
	Reason    string
}

type RelayData struct {
	TxSeq int64
	RxSeq int64
	Data  []byte
	Time  time.Time
}

var gTransporterID atomic.Int64

// wss://localhost/path
func NewClientTransporter(ctx context.Context, server string, token string, connect string, session int64) (*Transporter, int64, error) {
	header := make(http.Header)
	header.Set(`Authorization`, fmt.Sprintf(`%s %s`, authHeaderType, token))
	// TODO user agent

	conn, _, err := websocket.Dial(
		ctx, server,
		&websocket.DialOptions{
			HTTPHeader: header,
		},
	)
	if err != nil {
		return nil, 0, fmt.Errorf(`failed to dial server: %w`, err)
	}

	t := &Transporter{
		ctx:      ctx,
		conn:     conn,
		isClient: true,
		id:       gTransporterID.Add(1),
	}

	shouldCloseConn := true
	defer func() {
		if shouldCloseConn {
			conn.Close(websocket.StatusInternalError, "unknown")
		}
	}()

	// 创建新会话或者恢复会话
	if err := t.Write(&BeginSessionRequest{SessionID: session, Connect: connect}); err != nil {
		return nil, 0, fmt.Errorf("failed to write begin session request: %w", err)
	}

	var beginSessionResponse BeginSessionResponse
	if err := t.Read(&beginSessionResponse); err != nil {
		return nil, 0, fmt.Errorf(`failed to read begin session response: %w`, err)
	}

	if beginSessionResponse.SessionID == 0 {
		return nil, 0, fmt.Errorf(`actively closed: %s`, beginSessionResponse.Reason)
	}

	shouldCloseConn = false
	return t, beginSessionResponse.SessionID, nil
}

func NewServerTransporter(ctx context.Context, conn *websocket.Conn) *Transporter {
	return &Transporter{
		ctx:      ctx,
		conn:     conn,
		isClient: false,
		id:       gTransporterID.Add(1),
	}
}

func (t *Transporter) GetID() int64 {
	return t.id
}

// TODO: 正确关闭会话，发送主动关闭请求，防止死等。
func (t *Transporter) Close() error {
	return t.conn.Close(websocket.StatusNormalClosure, "close()")
}

func (t *Transporter) Read(out interface{}) error {
	// ctx := t.ctx
	// if t.isClient {
	// var cancel context.CancelFunc
	// ctx, cancel = context.WithTimeout(ctx, time.Second*30)
	// defer cancel()
	// }
	ty, r, err := t.conn.Reader(t.ctx)
	if err != nil {
		return fmt.Errorf(`error reading: %w`, err)
	}
	if ty != websocket.MessageBinary {
		return fmt.Errorf(`expect binary message while reading`)
	}
	if err := gob.NewDecoder(r).Decode(out); err != nil {
		return fmt.Errorf(`error decoding: %w`, err)
	}
	// 其实已经读完了，感觉是 bug，需要手动触发一下
	if n, err := io.Copy(io.Discard, r); err != nil || n != 0 {
		return fmt.Errorf(`error discarding: n=%d, err=%v`, n, err)
	}
	return nil
}

func (t *Transporter) Write(in interface{}) error {
	ctx, cancel := context.WithTimeout(t.ctx, time.Second*15)
	defer cancel()
	w, err := t.conn.Writer(ctx, websocket.MessageBinary)
	if err != nil {
		return fmt.Errorf(`error getting writer: %w`, err)
	}
	if err := gob.NewEncoder(w).Encode(in); err != nil {
		return fmt.Errorf(`error encoding: %w`, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf(`error closing writer: %w`, err)
	}
	return nil
}
