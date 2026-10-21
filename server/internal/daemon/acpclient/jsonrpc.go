// Package acpclient is a minimal Agent Client Protocol client (JSON-RPC 2.0
// over newline-delimited stdio). Version differences are isolated in
// adapter.go.
package acpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

type rpcMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
}

// RPCError is a JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("acp rpc error %d: %s", e.Code, e.Message) }

// Standard JSON-RPC error codes.
const (
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
)

// RequestHandler handles agent->client requests; NotificationHandler handles notifications.
type (
	RequestHandler      func(ctx context.Context, method string, params json.RawMessage) (any, *RPCError)
	NotificationHandler func(method string, params json.RawMessage)
)

type pendingCall struct {
	ch chan rpcMessage
}

// Conn is a bidirectional JSON-RPC connection.
type Conn struct {
	w       io.Writer
	wmu     sync.Mutex
	nextID  atomic.Int64
	mu      sync.Mutex
	pending map[int64]*pendingCall
	onReq   RequestHandler
	onNote  NotificationHandler
	done    chan struct{}
	err     error
	// Trace, when set, receives every raw frame ("in"/"out").
	Trace func(dir string, raw []byte)
}

// NewConn creates a connection; call Serve to start reading.
func NewConn(r io.Reader, w io.Writer, onReq RequestHandler, onNote NotificationHandler) *Conn {
	c := &Conn{w: w, pending: map[int64]*pendingCall{}, onReq: onReq, onNote: onNote, done: make(chan struct{})}
	go c.readLoop(r)
	return c
}

// Done is closed when the reader stops.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err returns the reader error after Done.
func (c *Conn) Err() error { return c.err }

func (c *Conn) write(m rpcMessage) error {
	m.JSONRPC = "2.0"
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if c.Trace != nil {
		c.Trace("out", raw)
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.w.Write(append(raw, '\n'))
	return err
}

// Call sends a request and waits for its response.
func (c *Conn) Call(ctx context.Context, method string, params any, out any) error {
	id := c.nextID.Add(1)
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	idRaw := json.RawMessage(fmt.Sprintf("%d", id))
	pc := &pendingCall{ch: make(chan rpcMessage, 1)}
	c.mu.Lock()
	c.pending[id] = pc
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	if err := c.write(rpcMessage{ID: &idRaw, Method: method, Params: raw}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return fmt.Errorf("acp: connection closed: %w", c.err)
	case m := <-pc.ch:
		if m.Error != nil {
			return m.Error
		}
		if out != nil && len(m.Result) > 0 {
			return json.Unmarshal(m.Result, out)
		}
		return nil
	}
}

// Notify sends a notification.
func (c *Conn) Notify(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.write(rpcMessage{Method: method, Params: raw})
}

func (c *Conn) readLoop(r io.Reader) {
	defer close(c.done)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 32*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		if c.Trace != nil {
			c.Trace("in", append([]byte(nil), line...))
		}
		var m rpcMessage
		if err := json.Unmarshal(line, &m); err != nil {
			continue
		}
		switch {
		case m.Method != "" && m.ID != nil:
			go c.handleRequest(m)
		case m.Method != "":
			if c.onNote != nil {
				c.onNote(m.Method, m.Params)
			}
		case m.ID != nil:
			var id int64
			if err := json.Unmarshal(*m.ID, &id); err != nil {
				continue
			}
			c.mu.Lock()
			pc := c.pending[id]
			c.mu.Unlock()
			if pc != nil {
				pc.ch <- m
			}
		}
	}
	c.err = sc.Err()
	if c.err == nil {
		c.err = io.EOF
	}
}

func (c *Conn) handleRequest(m rpcMessage) {
	var res any
	rerr := &RPCError{Code: CodeMethodNotFound, Message: "method not found: " + m.Method}
	if c.onReq != nil {
		res, rerr = c.onReq(context.Background(), m.Method, m.Params)
	}
	out := rpcMessage{ID: m.ID}
	if rerr != nil {
		out.Error = rerr
	} else {
		raw, err := json.Marshal(res)
		if err != nil {
			out.Error = &RPCError{Code: CodeInternal, Message: err.Error()}
		} else {
			out.Result = raw
		}
	}
	_ = c.write(out)
}
