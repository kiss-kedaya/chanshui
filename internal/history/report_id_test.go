package history

import (
	"database/sql"
	"path/filepath"
	"regexp"
	"testing"
)

func TestReportIDBackfillAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(schema); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("INSERT INTO detections(created_at,base_url,model,verdict_json) VALUES ('now','https://gateway.example/v1','model','{}'),('then','https://other.example','model','{}')"); e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	rows, e := s.List(50)
	if e != nil {
		t.Fatal(e)
	}
	valid := regexp.MustCompile("^[a-f0-9]{32}$")
	if len(rows) != 2 || !valid.MatchString(rows[0].ReportID) || rows[0].ReportID == rows[1].ReportID {
		t.Fatalf("bad IDs: %+v", rows)
	}
	first := rows[0].ReportID
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	rec, e := s.GetByReportID(first)
	if e != nil || rec.ID != rows[0].ID {
		t.Fatalf("restart lookup: %+v %v", rec, e)
	}
	again, _ := s.List(50)
	if again[0].ReportID != first {
		t.Fatal("backfill not stable")
	}
}

func TestReportIDUniqueAndCursorHistory(t *testing.T) {
	s := tempDB(t)
	ids := map[string]bool{}
	for i := 0; i < 55; i++ {
		id, e := s.Save(sampleRecord(i))
		if e != nil {
			t.Fatal(e)
		}
		rec, e := s.Get(id)
		if e != nil || rec.ReportID == "" || ids[rec.ReportID] {
			t.Fatalf("invalid unique ID: %v %v", rec, e)
		}
		ids[rec.ReportID] = true
	}
	page1, e := s.ListBefore(50, 0)
	if e != nil || len(page1) != 50 {
		t.Fatal(len(page1), e)
	}
	page2, e := s.ListBefore(50, page1[len(page1)-1].ID)
	if e != nil || len(page2) != 5 || page2[0].ID >= page1[len(page1)-1].ID {
		t.Fatal(page2, e)
	}
	r := sampleRecord(0)
	r.ReportID = page1[0].ReportID
	if _, e = s.Save(r); e == nil {
		t.Fatal("duplicate report ID accepted")
	}
}
