package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/collabos/collabos/v2/internal/wire"
)

// ErrNotFound is returned for unknown rows.
var ErrNotFound = errors.New("not found")

// DeviceRow is a registered device.
type DeviceRow struct {
	Name     string
	Owner    string
	LastSeen *time.Time
}

// Store persists devices and calls.
type Store interface {
	AddDevice(ctx context.Context, name, owner, tokenHash string) error
	DeviceByToken(ctx context.Context, tokenHash string) (DeviceRow, error)
	Devices(ctx context.Context) ([]DeviceRow, error)
	TouchDevice(ctx context.Context, name string) error
	SaveCall(ctx context.Context, c wire.Call) error
	Call(ctx context.Context, id string) (wire.Call, error)
	Calls(ctx context.Context, device string, limit int) ([]wire.Call, error)
	OpenCallsFor(ctx context.Context, device string) ([]wire.Call, error)
}

// NewToken returns a device token and its hash.
func NewToken() (token, hash string) {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token = "cdv_" + hex.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken hashes a device token for storage.
func HashToken(t string) string {
	s := sha256.Sum256([]byte(t))
	return hex.EncodeToString(s[:])
}

// ---------- memory (tests, local dev) ----------

// MemStore is an in-memory Store.
type MemStore struct {
	mu      sync.Mutex
	devices map[string]DeviceRow
	tokens  map[string]string
	calls   map[string]wire.Call
}

// NewMemStore returns an empty in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{devices: map[string]DeviceRow{}, tokens: map[string]string{}, calls: map[string]wire.Call{}}
}

func (m *MemStore) AddDevice(_ context.Context, name, owner, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for h, n := range m.tokens {
		if n == name {
			delete(m.tokens, h)
		}
	}
	m.devices[name] = DeviceRow{Name: name, Owner: owner}
	m.tokens[hash] = name
	return nil
}

func (m *MemStore) DeviceByToken(_ context.Context, hash string) (DeviceRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.tokens[hash]
	if !ok {
		return DeviceRow{}, ErrNotFound
	}
	return m.devices[n], nil
}

func (m *MemStore) Devices(context.Context) ([]DeviceRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []DeviceRow{}
	for _, d := range m.devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *MemStore) TouchDevice(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.devices[name]
	now := time.Now()
	d.LastSeen = &now
	m.devices[name] = d
	return nil
}

func (m *MemStore) SaveCall(_ context.Context, c wire.Call) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls[c.ID] = c
	return nil
}

func (m *MemStore) Call(_ context.Context, id string) (wire.Call, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.calls[id]
	if !ok {
		return c, ErrNotFound
	}
	return c, nil
}

func (m *MemStore) Calls(_ context.Context, device string, limit int) ([]wire.Call, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []wire.Call{}
	for _, c := range m.calls {
		if device == "" || c.FromDevice == device || c.ToDevice == device {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemStore) OpenCallsFor(_ context.Context, device string) ([]wire.Call, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []wire.Call{}
	for _, c := range m.calls {
		if c.ToDevice == device && !wire.Terminal(c.Status) {
			out = append(out, c)
		}
	}
	return out, nil
}

// ---------- Postgres ----------

// PGStore stores everything in Postgres (tables prefixed relay_).
type PGStore struct{ db *pgxpool.Pool }

const schema = `
CREATE TABLE IF NOT EXISTS relay_devices (
  name TEXT PRIMARY KEY,
  owner TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  last_seen TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS relay_calls (
  id TEXT PRIMARY KEY,
  from_device TEXT NOT NULL,
  to_device TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  body JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS relay_calls_to_status ON relay_calls (to_device, status);
CREATE INDEX IF NOT EXISTS relay_calls_created ON relay_calls (created_at DESC);
`

// OpenPG connects and creates the schema.
func OpenPG(ctx context.Context, url string) (*PGStore, error) {
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := p.Exec(ctx, schema); err != nil {
		p.Close()
		return nil, err
	}
	return &PGStore{db: p}, nil
}

func (s *PGStore) AddDevice(ctx context.Context, name, owner, hash string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO relay_devices (name, owner, token_hash) VALUES ($1,$2,$3)
		ON CONFLICT (name) DO UPDATE SET owner=EXCLUDED.owner, token_hash=EXCLUDED.token_hash`, name, owner, hash)
	return err
}

func (s *PGStore) DeviceByToken(ctx context.Context, hash string) (DeviceRow, error) {
	var d DeviceRow
	err := s.db.QueryRow(ctx, `SELECT name, owner, last_seen FROM relay_devices WHERE token_hash=$1`, hash).Scan(&d.Name, &d.Owner, &d.LastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

func (s *PGStore) Devices(ctx context.Context) ([]DeviceRow, error) {
	rows, err := s.db.Query(ctx, `SELECT name, owner, last_seen FROM relay_devices ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeviceRow{}
	for rows.Next() {
		var d DeviceRow
		if err := rows.Scan(&d.Name, &d.Owner, &d.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PGStore) TouchDevice(ctx context.Context, name string) error {
	_, err := s.db.Exec(ctx, `UPDATE relay_devices SET last_seen=now() WHERE name=$1`, name)
	return err
}

func (s *PGStore) SaveCall(ctx context.Context, c wire.Call) error {
	b, _ := json.Marshal(c)
	_, err := s.db.Exec(ctx, `INSERT INTO relay_calls (id, from_device, to_device, status, created_at, body) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, body=EXCLUDED.body`, c.ID, c.FromDevice, c.ToDevice, c.Status, c.CreatedAt, b)
	return err
}

func (s *PGStore) scan(rows pgx.Rows) ([]wire.Call, error) {
	defer rows.Close()
	out := []wire.Call{}
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var c wire.Call
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PGStore) Call(ctx context.Context, id string) (wire.Call, error) {
	rows, err := s.db.Query(ctx, `SELECT body FROM relay_calls WHERE id=$1`, id)
	if err != nil {
		return wire.Call{}, err
	}
	cs, err := s.scan(rows)
	if err != nil {
		return wire.Call{}, err
	}
	if len(cs) == 0 {
		return wire.Call{}, ErrNotFound
	}
	return cs[0], nil
}

func (s *PGStore) Calls(ctx context.Context, device string, limit int) ([]wire.Call, error) {
	rows, err := s.db.Query(ctx, `SELECT body FROM relay_calls WHERE $1='' OR from_device=$1 OR to_device=$1 ORDER BY created_at DESC LIMIT $2`, device, limit)
	if err != nil {
		return nil, err
	}
	return s.scan(rows)
}

func (s *PGStore) OpenCallsFor(ctx context.Context, device string) ([]wire.Call, error) {
	rows, err := s.db.Query(ctx, `SELECT body FROM relay_calls WHERE to_device=$1 AND status IN ('queued','running')`, device)
	if err != nil {
		return nil, err
	}
	return s.scan(rows)
}

// Close releases the pool.
func (s *PGStore) Close() { s.db.Close() }
