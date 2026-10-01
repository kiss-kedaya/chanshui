package history

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"regexp"
)

var reportIDPattern = regexp.MustCompile("^[a-f0-9]{32}$")

// NewReportID generates an opaque persistent ID for one report snapshot.
func NewReportID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("history: report ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// ValidReportID validates IDs without interpreting any of their content.
func ValidReportID(id string) bool { return reportIDPattern.MatchString(id) }

// migrateReportIDs is additive and preserves old numeric URLs and report contents.
func migrateReportIDs(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query("PRAGMA table_info(detections)")
	if err != nil {
		return err
	}
	exists := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		exists = exists || name == "report_id"
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec("ALTER TABLE detections ADD COLUMN report_id TEXT"); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("UPDATE detections SET report_id=lower(hex(randomblob(16))) WHERE report_id IS NULL OR report_id=''"); err != nil {
		return err
	}
	if _, err = tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS detections_report_id ON detections(report_id)"); err != nil {
		return err
	}
	return tx.Commit()
}

// GetByReportID loads a saved report independently of in-memory tasks or browser state.
func (s *Store) GetByReportID(reportID string) (*Record, error) {
	if !ValidReportID(reportID) {
		return nil, fmt.Errorf("history: invalid report ID")
	}
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history: store closed")
	}
	var id int64
	if err := s.db.QueryRow("SELECT id FROM detections WHERE report_id=?", reportID).Scan(&id); err != nil {
		return nil, fmt.Errorf("history: report lookup: %w", err)
	}
	return s.Get(id)
}
