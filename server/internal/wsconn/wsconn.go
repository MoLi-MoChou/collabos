// Package wsconn wraps a gorilla/websocket connection with a write pump.
package wsconn

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/collabos/collabos/server/pkg/protocol"
)

// Conn is a websocket with a buffered, single-writer send queue.
type Conn struct {
	ws     *websocket.Conn
	send   chan []byte
	once   sync.Once
	closed chan struct{}
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 25 * time.Second
)

// Wrap starts the write pump.
func Wrap(ws *websocket.Conn) *Conn {
	c := &Conn{ws: ws, send: make(chan []byte, 256), closed: make(chan struct{})}
	ws.SetReadLimit(4 << 20)
	_ = ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(pongWait)) })
	go c.writePump()
	return c
}

// Closed is closed when the connection ends.
func (c *Conn) Closed() <-chan struct{} { return c.closed }

// Send queues an envelope; it drops the connection if the queue is full.
func (c *Conn) Send(e protocol.Envelope) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	select {
	case c.send <- b:
	case <-c.closed:
	default:
		c.Close()
	}
}

// Close ends the connection once.
func (c *Conn) Close() {
	c.once.Do(func() {
		close(c.closed)
		_ = c.ws.Close()
	})
}

// Read blocks for the next envelope.
func (c *Conn) Read(ctx context.Context) (protocol.Envelope, error) {
	_, data, err := c.ws.ReadMessage()
	if err != nil {
		c.Close()
		return protocol.Envelope{}, err
	}
	_ = c.ws.SetReadDeadline(time.Now().Add(pongWait))
	return protocol.Decode(data)
}

func (c *Conn) writePump() {
	t := time.NewTicker(pingPeriod)
	defer t.Stop()
	for {
		select {
		case <-c.closed:
			return
		case b := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.TextMessage, b); err != nil {
				c.Close()
				return
			}
		case <-t.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.Close()
				return
			}
		}
	}
}
