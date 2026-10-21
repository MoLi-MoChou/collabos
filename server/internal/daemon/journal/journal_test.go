package journal

import (
	"path/filepath"
	"testing"

	"github.com/collabos/collabos/server/pkg/protocol"
)

func TestJournalLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "j.db")
	j, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		e, err := j.Append("t1", protocol.Msg(protocol.TypeDaemonProgressStream, map[string]int{"i": i}))
		if err != nil || e.Seq != int64(i+1) {
			t.Fatalf("append %d: seq=%d err=%v", i, e.Seq, err)
		}
	}
	if err := j.Ack(1); err != nil {
		t.Fatal(err)
	}
	_ = j.Close()

	// Survives restart; replays seq 2,3 in order.
	j, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	p, _ := j.Pending()
	if len(p) != 2 || p[0].Seq != 2 || p[1].Seq != 3 {
		t.Fatalf("pending = %+v", p)
	}
	ts, _ := j.Tasks()
	if len(ts) != 1 || ts[0] != "t1" {
		t.Fatalf("tasks = %v", ts)
	}
	// Server already has up to 10 (e.g. journal recreated): drop and skip ahead.
	if err := j.Sync(10); err != nil {
		t.Fatal(err)
	}
	if p, _ := j.Pending(); len(p) != 0 {
		t.Fatalf("expected empty after sync, got %d", len(p))
	}
	e, _ := j.Append("t2", protocol.Msg(protocol.TypeDaemonTaskCompleted, map[string]any{}))
	if e.Seq != 11 {
		t.Fatalf("seq after sync = %d", e.Seq)
	}
}
