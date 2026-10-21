// Package protocol defines the wire contract between server, UI clients and
// daemons (PRD chapter 9). It is the single source for the generated
// TypeScript types (tygo).
package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

// Envelope wraps every WebSocket frame.
type Envelope struct {
	Type      string          `json:"type"`
	TraceID   string          `json:"trace_id"`
	MsgID     string          `json:"msg_id"`
	Seq       int64           `json:"seq,omitempty"`
	AckSeq    int64           `json:"ack_seq,omitempty"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload" tstype:"unknown"`
}

// ErrInvalidEnvelope is returned when a frame misses required fields.
var ErrInvalidEnvelope = errors.New("protocol: invalid envelope")

// NewID returns a random 16-byte hex identifier.
func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// New builds an envelope with a fresh msg_id and timestamp.
func New(typ, traceID string, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	if traceID == "" {
		traceID = NewID()
	}
	return Envelope{
		Type:      typ,
		TraceID:   traceID,
		MsgID:     NewID(),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Payload:   raw,
	}, nil
}

// MustNew is New for payloads that are known to marshal.
func MustNew(typ, traceID string, payload any) Envelope {
	e, err := New(typ, traceID, payload)
	if err != nil {
		panic(err)
	}
	return e
}

// Decode parses and validates a frame.
func Decode(data []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return Envelope{}, err
	}
	if e.Type == "" || e.MsgID == "" {
		return Envelope{}, ErrInvalidEnvelope
	}
	return e, nil
}

// Into unmarshals the payload into v.
func (e Envelope) Into(v any) error {
	if len(e.Payload) == 0 {
		return ErrInvalidEnvelope
	}
	return json.Unmarshal(e.Payload, v)
}

// Msg is MustNew with a fresh trace id.
func Msg(typ string, payload any) Envelope { return MustNew(typ, "", payload) }

// MsgTrace builds an envelope that carries an existing trace_id.
func MsgTrace(typ, traceID string, payload any) Envelope { return MustNew(typ, traceID, payload) }
