package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"net/http"

	"nhooyr.io/websocket"
)

type Transporter struct {
	ctx context.Context

	conn *websocket.Conn
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
	Seq  int64
	Data []byte
}

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
		ctx:  ctx,
		conn: conn,
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
		ctx:  ctx,
		conn: conn,
	}
}

// TODO: 正确关闭会话，发送主动关闭请求，防止死等。
func (t *Transporter) Close() error {
	return t.conn.Close(websocket.StatusNormalClosure, "close()")
}

func (t *Transporter) Read(out interface{}) error {
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
	io.Copy(io.Discard, r)
	return nil
}

func (t *Transporter) Write(in interface{}) error {
	w, err := t.conn.Writer(t.ctx, websocket.MessageBinary)
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
