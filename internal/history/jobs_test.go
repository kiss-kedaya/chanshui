package history

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestTTLExpiryAndPhysicalCleanup(t *testing.T) {
	s := tempDB(t)
	id, _ := NewReportID()
	numeric, e := s.CreateJob(context.Background(), Record{ReportID: id, CreatedAt: time.Now().UTC().Format(time.RFC3339), BaseURL: "https://fixture.invalid", Model: "model", Verdict: json.RawMessage("{}")}, json.RawMessage("[]"), "hash", "")
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.Get(numeric)
	if e != nil || r.ExpiresAt-time.Now().Unix() < 86398 || r.Status != "queued" {
		t.Fatal(r, e)
	}
	if _, e = s.db.Exec("UPDATE detections SET expires_at=? WHERE id=?", time.Now().Unix()-1, numeric); e != nil {
		t.Fatal(e)
	}
	if _, e = s.GetByReportID(id); e == nil {
		t.Fatal("expired report readable")
	}
	if l, _ := s.List(50); len(l) != 0 {
		t.Fatal("expired listed")
	}
	n, e := s.PurgeExpired(context.Background(), time.Now())
	if n != 1 || e != nil {
		t.Fatal(n, e)
	}
	var count int
	s.db.QueryRow("SELECT count(*) FROM detections WHERE report_id=?", id).Scan(&count)
	if count != 0 {
		t.Fatal("expiry not physically deleted")
	}
}
func TestRestartMarksJobInterrupted(t *testing.T) {
	s := tempDB(t)
	id, _ := NewReportID()
	n, e := s.CreateJob(context.Background(), Record{ReportID: id, CreatedAt: time.Now().Format(time.RFC3339), Verdict: json.RawMessage("{}")}, json.RawMessage("[]"), "hash", "")
	if e != nil {
		t.Fatal(e)
	}
	s.RecoverInterrupted()
	r, e := s.Get(n)
	if e != nil || r.Status != "interrupted" {
		t.Fatal(r, e)
	}
}
