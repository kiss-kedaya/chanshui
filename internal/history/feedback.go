package history

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const auxiliarySchema = `
CREATE TABLE IF NOT EXISTS runtime_state(name TEXT PRIMARY KEY,value BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS feedback(id TEXT PRIMARY KEY,created_at INTEGER NOT NULL,kind TEXT NOT NULL,contact TEXT NOT NULL,message TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS feedback_created_at ON feedback(created_at);
`

// Feedback remains private; only the local administrative export reads contacts.
type Feedback struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"`
	Kind      string `json:"kind"`
	Contact   string `json:"contact"`
	Message   string `json:"message"`
}

func (s *Store) SaveFeedback(ctx context.Context, f Feedback) error {
	if !ValidReportID(f.ID) {
		return fmt.Errorf("invalid feedback ID")
	}
	_, e := s.db.ExecContext(ctx, "INSERT INTO feedback(id,created_at,kind,contact,message) VALUES(?,?,?,?,?)", f.ID, time.Now().Unix(), f.Kind, f.Contact, f.Message)
	return e
}
func (s *Store) ListFeedback(ctx context.Context) ([]Feedback, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT id,created_at,kind,contact,message FROM feedback ORDER BY created_at DESC LIMIT 10000")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Feedback{}
	for rows.Next() {
		var f Feedback
		if e = rows.Scan(&f.ID, &f.CreatedAt, &f.Kind, &f.Contact, &f.Message); e != nil {
			return nil, e
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
func (s *Store) SaveRuntimeState(ctx context.Context, name string, raw json.RawMessage) error {
	_, e := s.db.ExecContext(ctx, "INSERT INTO runtime_state(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value", name, []byte(raw))
	return e
}
func (s *Store) LoadRuntimeState(ctx context.Context, name string) (json.RawMessage, error) {
	var raw []byte
	e := s.db.QueryRowContext(ctx, "SELECT value FROM runtime_state WHERE name=?", name).Scan(&raw)
	return raw, e
}
