// Package history persists detection runs in SQLite (modernc.org/sqlite,
// pure Go, no CGO) and serves summary lists plus full detail rows.
package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS detections(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL,
  base_url TEXT NOT NULL, model TEXT NOT NULL,
  injection INTEGER, slope REAL,
  family TEXT, family_prob REAL, top_model TEXT, top_prob REAL,
  thinking_pass INTEGER, thinking_note TEXT,
  iq_score INTEGER, iq_max INTEGER DEFAULT 100,
  verdict_json TEXT
);`

// Store wraps the sqlite handle.
type Store struct {
	db *sql.DB
}

// Record mirrors the detections columns; Verdict holds the raw verdict JSON.
type Record struct {
	Questions    json.RawMessage `json:"-"`
	Status       string          `json:"status"`
	ExpiresAt    int64           `json:"expires_at"`
	Progress     json.RawMessage `json:"probes"`
	ReportID     string          `json:"report_id"`
	ID           int64           `json:"id"`
	CreatedAt    string          `json:"created_at"`
	BaseURL      string          `json:"base_url"`
	Model        string          `json:"model"`
	Injection    *int            `json:"injection,omitempty"`
	Slope        *float64        `json:"slope,omitempty"`
	Family       string          `json:"family,omitempty"`
	FamilyProb   *float64        `json:"family_prob,omitempty"`
	TopModel     string          `json:"top_model,omitempty"`
	TopProb      *float64        `json:"top_prob,omitempty"`
	ThinkingPass *bool           `json:"thinking_pass,omitempty"`
	ThinkingNote string          `json:"thinking_note,omitempty"`
	IQScore      *int            `json:"iq_score,omitempty"`
	IQMax        int             `json:"iq_max"`
	Verdict      json.RawMessage `json:"verdict,omitempty"`
}

// Summary is the sidebar list entry.
type Summary struct {
	Status       string `json:"status"`
	ExpiresAt    int64  `json:"expires_at"`
	ReportID     string `json:"report_id"`
	ID           int64  `json:"id"`
	CreatedAt    string `json:"created_at"`
	BaseURL      string `json:"base_url"`
	Model        string `json:"model"`
	IQScore      *int   `json:"iq_score,omitempty"`
	Family       string `json:"family,omitempty"`
	ThinkingPass *bool  `json:"thinking_pass,omitempty"`
}

// Open opens (or creates) the database at path and ensures the schema.
// ":memory:" keeps everything in RAM.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("history: create dir: %w", err)
			}
		}
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=secure_delete(ON)")
	if err != nil {
		return nil, fmt.Errorf("history: open %s: %w", path, err)
	}
	// modernc sqlite serializes writes; a single connection avoids
	// SQLITE_BUSY on concurrent saves.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("history: schema: %w", err)
	}
	if _, err := db.Exec(auxiliarySchema); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateReportIDs(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("history: report ID migration: %w", err)
	}
	if err := migrateJobs(db); err != nil {
		db.Close()
		return nil, err
	}
	if path != ":memory:" {
		db.SetMaxOpenConns(8)
		db.SetMaxIdleConns(8)
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Save inserts a record and returns its row id.
func (s *Store) Save(rec Record) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("history: store closed")
	}
	if rec.ReportID == "" {
		id, err := NewReportID()
		if err != nil {
			return 0, err
		}
		rec.ReportID = id
	}
	if !ValidReportID(rec.ReportID) {
		return 0, fmt.Errorf("history: invalid report ID")
	}
	verdict := ""
	if len(rec.Verdict) > 0 {
		verdict = string(rec.Verdict)
	}
	if rec.IQMax == 0 {
		rec.IQMax = 100
	}
	res, err := s.db.Exec(`INSERT INTO detections(
		report_id, created_at, base_url, model,
		injection, slope,
		family, family_prob, top_model, top_prob,
		thinking_pass, thinking_note,
		iq_score, iq_max, verdict_json,expires_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		rec.ReportID, rec.CreatedAt, rec.BaseURL, rec.Model,
		nullableInt(rec.Injection), nullableFloat(rec.Slope),
		rec.Family, nullableFloat(rec.FamilyProb), rec.TopModel, nullableFloat(rec.TopProb),
		nullableBool(rec.ThinkingPass), rec.ThinkingNote,
		nullableInt(rec.IQScore), rec.IQMax, verdict, time.Now().Add(JobTTL).Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("history: save: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("history: last id: %w", err)
	}
	return id, nil
}

// List returns up to limit summaries, newest (highest id) first.
func (s *Store) List(limit int) ([]Summary, error) { return s.ListBefore(limit, 0) }

// ListBefore paginates global history by its indexed numeric ID.
func (s *Store) ListBefore(limit int, before int64) ([]Summary, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history: store closed")
	}
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, report_id, status, expires_at, created_at, base_url, model,
		family, thinking_pass, iq_score
		FROM detections WHERE expires_at > ?`
	args := []any{time.Now().Unix()}
	if before > 0 {
		query += " AND id < ?"
		args = append(args, before)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("history: list: %w", err)
	}
	defer rows.Close()

	var out []Summary
	for rows.Next() {
		var sum Summary
		var family sql.NullString
		var pass sql.NullBool
		var score sql.NullInt64
		if err := rows.Scan(&sum.ID, &sum.ReportID, &sum.Status, &sum.ExpiresAt, &sum.CreatedAt, &sum.BaseURL, &sum.Model,
			&family, &pass, &score); err != nil {
			return nil, fmt.Errorf("history: scan: %w", err)
		}
		sum.Family = family.String
		if pass.Valid {
			v := pass.Bool
			sum.ThinkingPass = &v
		}
		if score.Valid {
			v := int(score.Int64)
			sum.IQScore = &v
		}
		out = append(out, sum)
	}
	return out, rows.Err()
}

// Get returns the full record for id, or an error when absent.
func (s *Store) Get(id int64) (*Record, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history: store closed")
	}
	rec := Record{ID: id}
	var progress, questions string
	row := s.db.QueryRow(`SELECT report_id, status, expires_at, progress_json, questions_json, created_at, base_url, model,
		injection, slope,
		family, family_prob, top_model, top_prob,
		thinking_pass, thinking_note,
		iq_score, iq_max, verdict_json
		FROM detections WHERE id = ? AND expires_at > ?`, id, time.Now().Unix())
	var (
		injection  sql.NullInt64
		slope      sql.NullFloat64
		family     sql.NullString
		familyProb sql.NullFloat64
		topModel   sql.NullString
		topProb    sql.NullFloat64
		pass       sql.NullBool
		note       sql.NullString
		iqScore    sql.NullInt64
		verdict    sql.NullString
	)
	if err := row.Scan(&rec.ReportID, &rec.Status, &rec.ExpiresAt, &progress, &questions, &rec.CreatedAt, &rec.BaseURL, &rec.Model,
		&injection, &slope,
		&family, &familyProb, &topModel, &topProb,
		&pass, &note,
		&iqScore, &rec.IQMax, &verdict); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("history: id %d not found", id)
		}
		return nil, fmt.Errorf("history: get %d: %w", id, err)
	}
	rec.Progress = json.RawMessage(progress)
	rec.Questions = json.RawMessage(questions)
	if injection.Valid {
		v := int(injection.Int64)
		rec.Injection = &v
	}
	if slope.Valid {
		v := slope.Float64
		rec.Slope = &v
	}
	rec.Family = family.String
	if familyProb.Valid {
		v := familyProb.Float64
		rec.FamilyProb = &v
	}
	rec.TopModel = topModel.String
	if topProb.Valid {
		v := topProb.Float64
		rec.TopProb = &v
	}
	if pass.Valid {
		v := pass.Bool
		rec.ThinkingPass = &v
	}
	rec.ThinkingNote = note.String
	if iqScore.Valid {
		v := int(iqScore.Int64)
		rec.IQScore = &v
	}
	if rec.IQMax == 0 {
		rec.IQMax = 100
	}
	if verdict.Valid {
		rec.Verdict = json.RawMessage(verdict.String)
	}
	return &rec, nil
}

func nullableInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableBool(p *bool) any {
	if p == nil {
		return nil
	}
	return *p
}
