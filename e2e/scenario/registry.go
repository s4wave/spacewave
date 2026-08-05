// Package scenario owns the compile-time catalog of cross-runtime user flows.
package scenario

import (
	"context"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/e2e/runtime"
)

// Status is the result state for one scenario and runtime pair.
type Status string

const (
	// StatusPass marks a selected scenario that completed successfully.
	StatusPass Status = "PASS"
	// StatusFail marks a selected scenario that failed in the runtime.
	StatusFail Status = "FAIL"
	// StatusSkip marks a selected scenario that the runtime cannot support.
	StatusSkip Status = "SKIP"
	// StatusNotRun marks a scenario that was filtered out by tags.
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

// Report contains one row for every scenario in the registry.
type Report struct {
	Rows []Row
}

// String renders the discovered-scenario report as a stable, line-oriented
// table suitable for CI logs and artifacts.
func (r Report) String() string {
	var b strings.Builder
	b.WriteString("SCENARIO\tRUNTIME\tSTATUS\tREASON\n")
	for _, row := range r.Rows {
		b.WriteString(row.Scenario)
		b.WriteByte('\t')
		b.WriteString(row.Runtime)
		b.WriteByte('\t')
		b.WriteString(string(row.Status))
		b.WriteByte('\t')
		b.WriteString(row.Reason)
		b.WriteByte('\n')
	}
	return b.String()
}

// Registry validates and owns an explicit scenario catalog.
type Registry struct {
	scenarios []Scenario
}

// NewRegistry constructs a scenario registry from an explicit catalog.
func NewRegistry(scenarios ...Scenario) *Registry {
	// Track names while copying the validated scenario catalog.
	seen := make(map[string]struct{}, len(scenarios))
	out := make([]Scenario, len(scenarios))
	for index, s := range scenarios {
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
		if _, ok := seen[s.Name]; ok {
			panic("duplicate scenario registration: " + s.Name)
		}
		seen[s.Name] = struct{}{}

		// Clone tags so later caller mutations cannot alter the registry.
		s.Tags = slices.Clone(s.Tags)
		out[index] = s
	}
	return &Registry{scenarios: out}
}

// All returns the registry scenarios in declaration order.
func (r *Registry) All() []Scenario {
	out := make([]Scenario, len(r.scenarios))
	copy(out, r.scenarios)
	for i := range out {
		out[i].Tags = slices.Clone(out[i].Tags)
	}
	return out
}

// ParseTags splits a comma-separated tag selector and removes duplicates.
func ParseTags(raw string) []string {
	// Track tags already accepted from the selector.
	seen := make(map[string]struct{})
	var tags []string

	// Normalize non-empty tags and discard duplicates.
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

	// Sort tags for stable selector behavior.
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

// Run executes selected scenarios in declaration order and emits NOT RUN rows
// for the rest. Declared session boundaries reset only the scenario that owns
// them; warm scenarios inherit the current runtime state.
func (r *Registry) Run(ctx context.Context, rt runtime.Runtime, tags []string) Report {
	scenarios := r.All()
	selectedScenarios := make([]Scenario, 0, len(scenarios))
	report := Report{Rows: make([]Row, len(scenarios))}

	// Initialize report rows and select scenarios matching the requested tags.
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

	// Reset session boundaries and execute each selected scenario.
	for _, s := range selectedScenarios {
		row := Row{Scenario: s.Name, Runtime: rt.Name()}
		if s.Session != runtime.SessionAny {
			if err := rt.ResetSession(s.Session); err != nil {
				row.Status = StatusFail
				row.Reason = errors.Wrapf(err, "reset %s", s.Session).Error()
				report.setRow(row)
				continue
			}
		}

		// Record failure, skip, or success for the scenario run.
		if err := s.Run(ctx, rt); err != nil {
			row.Status = StatusFail
			row.Reason = err.Error()
			if runtime.IsUnsupported(err) {
				row.Status = StatusSkip
				row.Reason = runtime.SkipReason(err)
			}
			report.setRow(row)
			continue
		}
		row.Status = StatusPass
		report.setRow(row)
	}
	return report
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
			return errors.Errorf("expected scenario row is missing: %s on %s", want.Scenario, want.Runtime)
		}
		if row.Status == StatusNotRun {
			return errors.Errorf("expected scenario %s on %s was not run: %s", want.Scenario, want.Runtime, row.Reason)
		}
		if row.Status == StatusFail {
			return errors.Errorf("expected scenario %s on %s failed: %s", want.Scenario, want.Runtime, row.Reason)
		}
	}
	return nil
}
