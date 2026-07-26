// Package scenario owns the compile-time catalog of cross-runtime user flows.
package scenario

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/s4wave/spacewave/e2e/runtime"
)

// Status is the result state for one scenario and runtime pair.
type Status string

const (
	StatusPass   Status = "PASS"
	StatusFail   Status = "FAIL"
	StatusSkip   Status = "SKIP"
	StatusNotRun Status = "NOT RUN"
)

// Scenario is one user flow with stable tags for CI slicing.
type Scenario struct {
	Name    string
	Tags    []string
	Session runtime.SessionRequirement
	Run     func(context.Context, runtime.Runtime) error
}

// Expectation identifies a scenario/runtime row required by an aggregate job.
type Expectation struct {
	Scenario string
	Runtime  string
}

// Row is the report record for one scenario/runtime pair.
type Row struct {
	Scenario string
	Runtime  string
	Status   Status
	Reason   string
}

// Report contains one row for every registered scenario on the runtime.
type Report struct {
	Rows []Row
}

// String renders the discovered-scenario report as a stable, line-oriented
// table suitable for CI logs and artifacts.
func (r Report) String() string {
	var b strings.Builder
	b.WriteString("SCENARIO\tRUNTIME\tSTATUS\tREASON\n")
	for _, row := range r.Rows {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", row.Scenario, row.Runtime, row.Status, row.Reason)
	}
	return b.String()
}

var (
	registryMu sync.RWMutex
	registry   []Scenario
)

// Register adds a compile-time scenario registration to the catalog.
func Register(s Scenario) {
	if s.Name == "" {
		panic("scenario name is required")
	}
	if s.Run == nil {
		panic("scenario run function is required: " + s.Name)
	}
	if !s.Session.Valid() {
		panic("invalid session requirement for scenario " + s.Name + ": " + string(s.Session))
	}
	if len(s.Tags) == 0 {
		panic("scenario tags are required: " + s.Name)
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	for _, existing := range registry {
		if existing.Name == s.Name {
			panic("duplicate scenario registration: " + s.Name)
		}
	}
	s.Tags = slices.Clone(s.Tags)
	registry = append(registry, s)
}

// All returns the registered scenarios in declaration order.
func All() []Scenario {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Scenario, len(registry))
	copy(out, registry)
	for i := range out {
		out[i].Tags = slices.Clone(out[i].Tags)
	}
	return out
}

// ParseTags splits a comma-separated tag selector and removes duplicates.
func ParseTags(raw string) []string {
	seen := make(map[string]struct{})
	var tags []string
	for tag := range strings.SplitSeq(raw, ",") {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	return tags
}

func selected(s Scenario, tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	for _, want := range tags {
		if slices.Contains(s.Tags, want) {
			return true
		}
	}
	return false
}

// Run executes selected registrations and emits NOT RUN rows for the rest.
// Selected scenarios are stably grouped by session boundary so warm scenarios
// share one session while declared boundaries remain explicit and ordered.
func Run(ctx context.Context, rt runtime.Runtime, tags []string) Report {
	scenarios := All()
	selectedScenarios := make([]Scenario, 0, len(scenarios))
	report := Report{Rows: make([]Row, len(scenarios))}
	for index, s := range scenarios {
		report.Rows[index] = Row{
			Scenario: s.Name,
			Runtime:  rt.Name(),
			Status:   StatusNotRun,
			Reason:   "tag not selected",
		}
		if selected(s, tags) {
			selectedScenarios = append(selectedScenarios, s)
		}
	}
	slices.SortStableFunc(selectedScenarios, func(a, b Scenario) int {
		return sessionRank(a.Session) - sessionRank(b.Session)
	})

	for _, s := range selectedScenarios {
		row := Row{Scenario: s.Name, Runtime: rt.Name()}
		if s.Session != runtime.SessionAny {
			if err := rt.ResetSession(s.Session); err != nil {
				row.Status = StatusFail
				row.Reason = fmt.Errorf("reset %s: %w", s.Session, err).Error()
				report.setRow(row)
				continue
			}
		}
		if err := s.Run(ctx, rt); err != nil {
			row.Status = StatusFail
			row.Reason = err.Error()
			if runtime.IsUnsupported(err) {
				row.Status = StatusSkip
				row.Reason = runtime.SkipReason(err)
			}
		} else {
			row.Status = StatusPass
		}
		report.setRow(row)
	}
	return report
}

func sessionRank(requirement runtime.SessionRequirement) int {
	switch requirement {
	case runtime.SessionFreshInstall:
		return 0
	case runtime.SessionFresh:
		return 1
	default:
		return 2
	}
}

func (r *Report) setRow(row Row) {
	for index := range r.Rows {
		if r.Rows[index].Scenario == row.Scenario && r.Rows[index].Runtime == row.Runtime {
			r.Rows[index] = row
			return
		}
	}
}

// Row returns a report row, or a zero row when it is absent.
func (r Report) Row(scenario string, runtimeName string) Row {
	for _, row := range r.Rows {
		if row.Scenario == scenario && row.Runtime == runtimeName {
			return row
		}
	}
	return Row{}
}

// ValidateExpected fails when an aggregate-required row is absent, unselected,
// or failed. SKIP is a valid coverage row when the runtime declares a control
// unsupported and includes the reason in the report.
func (r Report) ValidateExpected(expected []Expectation) error {
	for _, want := range expected {
		row := r.Row(want.Scenario, want.Runtime)
		if row.Scenario == "" {
			return fmt.Errorf("expected scenario row is missing: %s on %s", want.Scenario, want.Runtime)
		}
		if row.Status == StatusNotRun {
			return fmt.Errorf("expected scenario %s on %s was not run: %s", want.Scenario, want.Runtime, row.Reason)
		}
		if row.Status == StatusFail {
			return fmt.Errorf("expected scenario %s on %s failed: %s", want.Scenario, want.Runtime, row.Reason)
		}
	}
	return nil
}

func registrySnapshot() []Scenario {
	return All()
}

func setRegistryForTest(scenarios []Scenario) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = slices.Clone(scenarios)
}
