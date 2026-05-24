package selfenrollmentrun

import "testing"

type testSummary struct {
	ids           []string
	generationKey string
	count         uint32
}

func (s *testSummary) GetIDs() []string {
	return s.ids
}

func (s *testSummary) GetGenerationKey() string {
	return s.generationKey
}

func (s *testSummary) GetCount() uint32 {
	return s.count
}

func TestNewRequestSnapshotsSummary(t *testing.T) {
	summary := &testSummary{
		ids:           []string{"so-1"},
		generationKey: "gen-1",
		count:         1,
	}
	req := NewRequest(summary)
	if req == nil {
		t.Fatal("expected request")
	}
	if req.GenerationKey != "gen-1" {
		t.Fatalf("generation key = %q, want gen-1", req.GenerationKey)
	}
	if len(req.IDs) != 1 || req.IDs[0] != "so-1" {
		t.Fatalf("ids = %#v, want [so-1]", req.IDs)
	}

	summary.ids[0] = "changed"
	if req.IDs[0] != "so-1" {
		t.Fatalf("request ids changed after summary mutation: %#v", req.IDs)
	}
}

func TestNewRequestSkipsEmptySummary(t *testing.T) {
	if req := NewRequest(nil); req != nil {
		t.Fatalf("nil summary request = %#v, want nil", req)
	}
	if req := NewRequest(&testSummary{}); req != nil {
		t.Fatalf("empty summary request = %#v, want nil", req)
	}
}

func TestShouldAutoStart(t *testing.T) {
	summary := &testSummary{
		ids:           []string{"so-1"},
		generationKey: "gen-1",
		count:         1,
	}

	if !ShouldAutoStart(summary, "", true, 1) {
		t.Fatal("expected unlocked pending summary to auto-start")
	}
	if ShouldAutoStart(summary, "", false, 1) {
		t.Fatal("expected missing routine to block auto-start")
	}
	if ShouldAutoStart(summary, "gen-1", true, 1) {
		t.Fatal("expected skipped generation to block auto-start")
	}
	if ShouldAutoStart(summary, "", true, 0) {
		t.Fatal("expected locked account to block auto-start")
	}
	if ShouldAutoStart(nil, "", true, 1) {
		t.Fatal("expected nil summary to block auto-start")
	}
}
