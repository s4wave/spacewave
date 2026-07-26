package scenario

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/e2e/runtime"
)

type testRuntime struct {
	name string
}

func (r testRuntime) Name() string                              { return r.name }
func (r testRuntime) OpenRoute(string) error                    { return nil }
func (r testRuntime) ClickControl(string) error                 { return nil }
func (r testRuntime) DoubleClickContent(string) error           { return nil }
func (r testRuntime) Type(string, string) error                 { return nil }
func (r testRuntime) UploadFile(string, runtime.File) error     { return nil }
func (r testRuntime) MoveContent(string, string) error          { return nil }
func (r testRuntime) DeleteSpace() error                        { return nil }
func (r testRuntime) ExpectVisible(string) error                { return nil }
func (r testRuntime) ExpectAbsent(string) error                 { return nil }
func (r testRuntime) ExpectRoute(string) error                  { return nil }
func (r testRuntime) WaitForEvent(runtime.Event) error          { return nil }
func (r testRuntime) OpenSecondTab(string) (runtime.Tab, error) { return nil, nil }
func (r testRuntime) BackgroundTab(runtime.Tab) error           { return nil }
func (r testRuntime) ReloadPage() error                         { return nil }
func (r testRuntime) RestartWorkerHost() error                  { return nil }
func (r testRuntime) ResetSession(requirement runtime.SessionRequirement) error {
	return nil
}

func TestRunnerUsesDeclaredOrderAndSessionBoundaries(t *testing.T) {
	var calls []string
	reset := func(requirement runtime.SessionRequirement) error {
		calls = append(calls, "reset:"+string(requirement))
		return nil
	}
	rt := &recordingRuntime{testRuntime: testRuntime{name: "devwasm"}, reset: reset}
	registry := NewRegistry(
		Scenario{Name: "warm-after-install", Tags: []string{"drive"}, Session: runtime.SessionAny, Run: func(context.Context, runtime.Runtime) error {
			calls = append(calls, "warm-after-install")
			return nil
		}},
		Scenario{Name: "fresh-page", Tags: []string{"drive"}, Session: runtime.SessionFresh, Run: func(context.Context, runtime.Runtime) error {
			calls = append(calls, "fresh-page")
			return nil
		}},
		Scenario{Name: "fresh-install", Tags: []string{"drive"}, Session: runtime.SessionFreshInstall, Run: func(context.Context, runtime.Runtime) error {
			calls = append(calls, "fresh-install")
			return nil
		}},
		Scenario{Name: "warm-before-install", Tags: []string{"drive"}, Session: runtime.SessionAny, Run: func(context.Context, runtime.Runtime) error {
			calls = append(calls, "warm-before-install")
			return nil
		}},
	)

	report := registry.Run(context.Background(), rt, []string{"drive"})
	if got, want := calls, []string{
		"warm-after-install",
		"reset:fresh-session",
		"fresh-page",
		"reset:fresh-install",
		"fresh-install",
		"warm-before-install",
	}; !slices.Equal(got, want) {
		t.Fatalf("runner calls = %v, want %v", got, want)
	}
	if len(report.Rows) != 4 {
		t.Fatalf("report rows = %d, want 4", len(report.Rows))
	}
}

func TestDriveScenarioBoundariesCoverFirstUseAndRecovery(t *testing.T) {
	scenarios := NewRegistry(DriveScenarios()...).All()
	want := map[string]runtime.SessionRequirement{
		"drive.first-use.landing":     runtime.SessionFreshInstall,
		"drive.first-use.direct":      runtime.SessionFreshInstall,
		"drive.upload-crash-recovery": runtime.SessionFresh,
		"drive.row-move":              runtime.SessionFreshInstall,
	}
	for name, requirement := range want {
		var found bool
		for _, scenario := range scenarios {
			if scenario.Name != name {
				continue
			}
			found = true
			if scenario.Session != requirement {
				t.Fatalf("%s session = %q, want %q", name, scenario.Session, requirement)
			}
			break
		}
		if !found {
			t.Fatalf("scenario %q is not registered", name)
		}
	}
}

type recordingRuntime struct {
	testRuntime
	reset func(runtime.SessionRequirement) error
}

func (r *recordingRuntime) ResetSession(requirement runtime.SessionRequirement) error {
	return r.reset(requirement)
}

func TestReportIncludesNotRunRowsAndValidatesExpectedScenarios(t *testing.T) {
	registry := NewRegistry(
		Scenario{Name: "drive.first-use", Tags: []string{"drive", "first-use"}, Run: func(context.Context, runtime.Runtime) error { return nil }},
		Scenario{Name: "drive.upload", Tags: []string{"drive", "upload"}, Run: func(context.Context, runtime.Runtime) error { return nil }},
	)

	report := registry.Run(context.Background(), testRuntime{name: "devwasm"}, []string{"first-use"})
	if got := report.Row("drive.first-use", "devwasm"); got.Status != StatusPass {
		t.Fatalf("first-use row = %+v", got)
	}
	if got := report.Row("drive.upload", "devwasm"); got.Status != StatusNotRun {
		t.Fatalf("upload row = %+v", got)
	}
	if err := report.ValidateExpected([]Expectation{{Scenario: "drive.first-use", Runtime: "devwasm"}}); err != nil {
		t.Fatalf("validate expected: %v", err)
	}
}

func TestReportRejectsMissingExpectedScenario(t *testing.T) {
	registry := NewRegistry(
		Scenario{Name: "drive.first-use", Tags: []string{"drive"}, Run: func(context.Context, runtime.Runtime) error { return nil }},
	)

	report := registry.Run(context.Background(), testRuntime{name: "devwasm"}, []string{"drive"})
	err := report.ValidateExpected([]Expectation{{Scenario: "drive.navigation", Runtime: "devwasm"}})
	if err == nil || !strings.Contains(err.Error(), "drive.navigation") {
		t.Fatalf("expected missing scenario error, got %v", err)
	}
}

func TestRunnerRecordsSkipAndFailureReasons(t *testing.T) {
	registry := NewRegistry(
		Scenario{Name: "unsupported", Tags: []string{"drive"}, Run: func(context.Context, runtime.Runtime) error {
			return runtime.Unsupported("reload page", "devwasm cannot reload")
		}},
		Scenario{Name: "failed", Tags: []string{"drive"}, Run: func(context.Context, runtime.Runtime) error { return errors.New("assertion failed") }},
	)

	report := registry.Run(context.Background(), testRuntime{name: "devwasm"}, []string{"drive"})
	if got := report.Row("unsupported", "devwasm"); got.Status != StatusSkip || got.Reason != "devwasm cannot reload" {
		t.Fatalf("unsupported row = %+v", got)
	}
	if got := report.Row("failed", "devwasm"); got.Status != StatusFail || got.Reason != "assertion failed" {
		t.Fatalf("failed row = %+v", got)
	}
}
