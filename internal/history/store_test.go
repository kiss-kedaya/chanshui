package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func tempDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sampleRecord(i int) Record {
	verdict, _ := json.Marshal(map[string]any{
		"pass":        i%2 == 0,
		"reason":      "测试理由",
		"think_len":   800 + i,
		"sig_len":     208,
		"iq_score":    60 + i,
		"injection":   516,
		"family":      "claude",
		"family_prob": 0.91,
	})
	return Record{
		CreatedAt:    "2026-10-01T10:00:00Z",
		BaseURL:      "https://gw.example.com",
		Model:        "claude-sonnet-4-5",
		Injection:    intPtr(516 + i),
		Slope:        floatPtr(0.738),
		Family:       "claude",
		FamilyProb:   floatPtr(0.91),
		TopModel:     "claude-sonnet-4-5-20250929",
		TopProb:      floatPtr(0.87),
		ThinkingPass: boolPtr(i%2 == 0),
		ThinkingNote: "思考文本正常回传",
		IQScore:      intPtr(60 + i),
		Verdict:      verdict,
	}
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }
func boolPtr(v bool) *bool        { return &v }

func TestSaveAndListNewestFirst(t *testing.T) {
	s := tempDB(t)
	for i := 0; i < 3; i++ {
		id, err := s.Save(sampleRecord(i))
		if err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
		if id != int64(i+1) {
			t.Errorf("Save %d: id = %d, want %d", i, id, i+1)
		}
	}
	list, err := s.List(50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(List) = %d, want 3", len(list))
	}
	if list[0].ID != 3 || list[1].ID != 2 || list[2].ID != 1 {
		t.Errorf("List not newest first: %v %v %v", list[0].ID, list[1].ID, list[2].ID)
	}
	rec := sampleRecord(2)
	if list[0].Model != rec.Model {
		t.Errorf("list[0].Model = %q, want %q", list[0].Model, rec.Model)
	}
	if list[0].BaseURL != rec.BaseURL {
		t.Errorf("list[0].BaseURL = %q, want %q", list[0].BaseURL, rec.BaseURL)
	}
	if list[0].IQScore == nil || *list[0].IQScore != 62 {
		t.Errorf("list[0].IQScore = %v, want 62", list[0].IQScore)
	}
	if list[0].Family != "claude" {
		t.Errorf("list[0].Family = %q", list[0].Family)
	}
	if list[0].ThinkingPass == nil || !*list[0].ThinkingPass {
		t.Errorf("list[0].ThinkingPass = %v, want true", list[0].ThinkingPass)
	}
}

func TestGetFullFields(t *testing.T) {
	s := tempDB(t)
	want := sampleRecord(2)
	if _, err := s.Save(sampleRecord(0)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	id, err := s.Save(want)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != id {
		t.Errorf("ID = %d, want %d", got.ID, id)
	}
	if got.CreatedAt != want.CreatedAt {
		t.Errorf("CreatedAt = %q", got.CreatedAt)
	}
	if got.BaseURL != want.BaseURL || got.Model != want.Model {
		t.Errorf("BaseURL/Model = %q/%q", got.BaseURL, got.Model)
	}
	if got.Injection == nil || *got.Injection != 518 {
		t.Errorf("Injection = %v, want 518", got.Injection)
	}
	if got.Slope == nil || *got.Slope != 0.738 {
		t.Errorf("Slope = %v, want 0.738", got.Slope)
	}
	if got.Family != "claude" || got.FamilyProb == nil || *got.FamilyProb != 0.91 {
		t.Errorf("Family/Prob = %q/%v", got.Family, got.FamilyProb)
	}
	if got.TopModel != want.TopModel || got.TopProb == nil || *got.TopProb != 0.87 {
		t.Errorf("TopModel/Prob = %q/%v", got.TopModel, got.TopProb)
	}
	if got.ThinkingPass == nil || !*got.ThinkingPass {
		t.Errorf("ThinkingPass = %v, want true", got.ThinkingPass)
	}
	if got.ThinkingNote != "思考文本正常回传" {
		t.Errorf("ThinkingNote = %q", got.ThinkingNote)
	}
	if got.IQScore == nil || *got.IQScore != 62 {
		t.Errorf("IQScore = %v, want 62", got.IQScore)
	}
	var v map[string]any
	if err := json.Unmarshal(got.Verdict, &v); err != nil {
		t.Fatalf("Verdict not valid JSON: %v", err)
	}
	if v["reason"] != "测试理由" {
		t.Errorf("Verdict reason = %v", v["reason"])
	}
}

func TestVerdictJSONRoundTrip(t *testing.T) {
	s := tempDB(t)
	rec := sampleRecord(0)
	rec.Verdict = json.RawMessage(`{"nested":{"deep":[1,2,3]},"pass":true}`)
	id, err := s.Save(rec)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got.Verdict) != string(rec.Verdict) {
		t.Errorf("Verdict round-trip mismatch:\n got %s\nwant %s", got.Verdict, rec.Verdict)
	}
}

func TestGetMissing(t *testing.T) {
	s := tempDB(t)
	if _, err := s.Get(999); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestListLimit(t *testing.T) {
	s := tempDB(t)
	for i := 0; i < 5; i++ {
		if _, err := s.Save(sampleRecord(i)); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	list, err := s.List(2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(List) = %d, want 2", len(list))
	}
	if list[0].ID != 5 {
		t.Errorf("list[0].ID = %d, want 5 (newest first)", list[0].ID)
	}
}

func TestOpenInMemory(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open :memory:: %v", err)
	}
	defer s.Close()
	if _, err := s.Save(sampleRecord(0)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	list, err := s.List(10)
	if err != nil || len(list) != 1 {
		t.Fatalf("List after in-memory save: %v %v", list, err)
	}
}

func TestOpenCreatesFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "history")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "sub", "db.sqlite")
	if _, err = Open(path); err != nil {
		t.Fatalf("Open nested path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("db file not created: %v", err)
	}
}
