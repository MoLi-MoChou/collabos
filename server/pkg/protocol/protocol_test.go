package protocol

import (
	"encoding/json"
	"testing"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	in := DelegationDispatch{TaskID: "t1", Prompt: "hi", Grant: Grant{Resources: []string{"repo:**"}, Actions: []string{"read"}}}
	e, err := New(TypeDelegationDispatch, "", in)
	if err != nil {
		t.Fatal(err)
	}
	if e.TraceID == "" || e.MsgID == "" || e.Timestamp == "" {
		t.Fatalf("missing generated fields: %+v", e)
	}
	raw, _ := json.Marshal(e)
	got, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	var out DelegationDispatch
	if err := got.Into(&out); err != nil {
		t.Fatal(err)
	}
	if out.TaskID != "t1" || out.Grant.Actions[0] != "read" {
		t.Fatalf("payload mismatch: %+v", out)
	}
}

func TestDecodeRejectsInvalid(t *testing.T) {
	cases := []string{`{}`, `{"type":"X"}`, `{"msg_id":"1"}`, `not json`}
	for _, c := range cases {
		if _, err := Decode([]byte(c)); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}

func TestIntoEmptyPayload(t *testing.T) {
	e := Envelope{Type: "X", MsgID: "1"}
	var v map[string]any
	if err := e.Into(&v); err == nil {
		t.Fatal("expected error for empty payload")
	}
}
