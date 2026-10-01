package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const JobTTL = 24 * time.Hour

func migrateJobs(db *sql.DB) error {
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	rows, e := tx.Query("PRAGMA table_info(detections)")
	if e != nil {
		return e
	}
	cols := map[string]bool{}
	for rows.Next() {
		var a, c, d int
		var name, kind string
		var val any
		if e := rows.Scan(&a, &name, &kind, &c, &val, &d); e != nil {
			rows.Close()
			return e
		}
		cols[name] = true
	}
	rows.Close()
	for _, col := range []struct{ name, kind string }{{"status", "TEXT NOT NULL DEFAULT 'done'"}, {"expires_at", "INTEGER NOT NULL DEFAULT 0"}, {"progress_json", "TEXT NOT NULL DEFAULT '[]'"}, {"control_hash", "TEXT NOT NULL DEFAULT ''"}, {"questions_json", "TEXT NOT NULL DEFAULT '[]'"}} {
		if !cols[col.name] {
			if _, e = tx.Exec("ALTER TABLE detections ADD COLUMN " + col.name + " " + col.kind); e != nil {
				return e
			}
		}
	}
	// Existing reports inherit 24 hours from their creation, not from each restart.
	if _, e = tx.Exec("UPDATE detections SET expires_at=COALESCE(unixepoch(created_at),unixepoch())+86400 WHERE expires_at=0"); e != nil {
		return e
	}
	if _, e = tx.Exec("CREATE INDEX IF NOT EXISTS detections_expiry ON detections(expires_at)"); e != nil {
		return e
	}
	return tx.Commit()
}

// CreateJob commits the public job ID before it is admitted to a worker.
func (s *Store) CreateJob(ctx context.Context, r Record, progress json.RawMessage, controlHash string) (int64, error) {
	if !ValidReportID(r.ReportID) {
		return 0, fmt.Errorf("invalid job ID")
	}
	expires := time.Now().Add(JobTTL).Unix()
	res, e := s.db.ExecContext(ctx, "INSERT INTO detections(report_id,created_at,base_url,model,status,expires_at,progress_json,control_hash,verdict_json,questions_json,iq_max) VALUES(?,?,?,?,?,?,?,?,?,?,100)", r.ReportID, r.CreatedAt, r.BaseURL, r.Model, "queued", expires, string(progress), controlHash, string(r.Verdict), string(r.Questions))
	if e != nil {
		return 0, e
	}
	return res.LastInsertId()
}
func (s *Store) UpdateJob(ctx context.Context, r Record, progress json.RawMessage) error {
	res, e := s.db.ExecContext(ctx, "UPDATE detections SET status=?,progress_json=?,verdict_json=?,iq_score=?,thinking_pass=?,family=? WHERE report_id=? AND expires_at>?", r.Status, string(progress), string(r.Verdict), nullableInt(r.IQScore), nullableBool(r.ThinkingPass), r.Family, r.ReportID, time.Now().Unix())
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e == nil && n == 0 {
		return fmt.Errorf("job expired or missing")
	}
	return e
}
func (s *Store) PurgeExpired(ctx context.Context, now time.Time) (int64, error) {
	res, e := s.db.ExecContext(ctx, "DELETE FROM detections WHERE id IN (SELECT id FROM detections WHERE expires_at<=? ORDER BY expires_at LIMIT 500)", now.Unix())
	if e != nil {
		return 0, e
	}
	return res.RowsAffected()
}

// RecoverInterrupted never replays billed requests or persists keys after restart.
func (s *Store) RecoverInterrupted() error {
	_, e := s.db.Exec("UPDATE detections SET status='interrupted' WHERE status IN ('queued','running')")
	return e
}
func (s *Store) ControlHash(id string) (string, error) {
	var h string
	e := s.db.QueryRow("SELECT control_hash FROM detections WHERE report_id=? AND expires_at>?", id, time.Now().Unix()).Scan(&h)
	return h, e
}
