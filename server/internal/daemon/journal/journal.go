// Package journal is the daemon's durable outbox (PRD 7.3): every frame the
// daemon reports is persisted with a monotonically increasing seq before it
// is sent, deleted once the server acknowledges it, and replayed in order
// after a reconnect or restart.
package journal

import (
	"database/sql"
	"encoding/json"
	"sync"

	_ "modernc.org/sqlite" // sqlite driver

	"github.com/collabos/collabos/server/pkg/protocol"
)

// Journal is a SQLite-backed outbox.
type Journal struct {
	db *sql.DB
	mu sync.Mutex
}

// Open opens (or creates) the journal at path.
func Open(path string) (*Journal, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS outbox (seq INTEGER PRIMARY KEY, task_id TEXT NOT NULL, frame TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v INTEGER NOT NULL);
INSERT OR IGNORE INTO meta (k, v) VALUES ('next_seq', 1);`); err != nil {
		return nil, err
	}
	return &Journal{db: db}, nil
}

// Close closes the database.
func (j *Journal) Close() error { return j.db.Close() }

// Append assigns the next seq to e, persists it and returns it.
func (j *Journal) Append(taskID string, e protocol.Envelope) (protocol.Envelope, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	tx, err := j.db.Begin()
	if err != nil {
		return e, err
	}
	defer func() { _ = tx.Rollback() }()
	var seq int64
	if err := tx.QueryRow(`SELECT v FROM meta WHERE k = 'next_seq'`).Scan(&seq); err != nil {
		return e, err
	}
	e.Seq = seq
	b, err := json.Marshal(e)
	if err != nil {
		return e, err
	}
	if _, err := tx.Exec(`INSERT INTO outbox (seq, task_id, frame) VALUES (?, ?, ?)`, seq, taskID, string(b)); err != nil {
		return e, err
	}
	if _, err := tx.Exec(`UPDATE meta SET v = ? WHERE k = 'next_seq'`, seq+1); err != nil {
		return e, err
	}
	return e, tx.Commit()
}

// Ack deletes every frame with seq <= upTo.
func (j *Journal) Ack(upTo int64) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	_, err := j.db.Exec(`DELETE FROM outbox WHERE seq <= ?`, upTo)
	return err
}

// Sync aligns with the server after a handshake: frames the server already
// has are dropped and next_seq is moved past the server's last ack (so a
// recreated journal never reuses acknowledged seqs).
func (j *Journal) Sync(serverAcked int64) error {
	if err := j.Ack(serverAcked); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	_, err := j.db.Exec(`UPDATE meta SET v = MAX(v, ?) WHERE k = 'next_seq'`, serverAcked+1)
	return err
}

// Pending returns unacknowledged frames in seq order.
func (j *Journal) Pending() ([]protocol.Envelope, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	rows, err := j.db.Query(`SELECT frame FROM outbox ORDER BY seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []protocol.Envelope
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		var e protocol.Envelope
		if err := json.Unmarshal([]byte(s), &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Tasks lists task ids that still have unacknowledged frames.
func (j *Journal) Tasks() ([]string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	rows, err := j.db.Query(`SELECT DISTINCT task_id FROM outbox WHERE task_id <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Unfinished lists tasks with unacknowledged frames but no completion frame:
// after a daemon restart these runs are gone and must be reported as lost.
func (j *Journal) Unfinished(completedType string) ([]string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	rows, err := j.db.Query(`SELECT DISTINCT task_id FROM outbox o WHERE task_id <> ''
		AND NOT EXISTS (SELECT 1 FROM outbox c WHERE c.task_id = o.task_id AND json_extract(c.frame, '$.type') = ?)`, completedType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
