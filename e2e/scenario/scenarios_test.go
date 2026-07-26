//go:build e2e_scenario && !js

package scenario

import (
	"context"
	"os"
	"testing"

	"github.com/s4wave/spacewave/e2e/runtime/devwasm"
	"github.com/s4wave/spacewave/e2e/wasm"
	"github.com/sirupsen/logrus"
)

// TIER: pr
func TestMain(m *testing.M) {
	if !wasm.E2EWasmEnabled() {
		logrus.NewEntry(logrus.New()).Info("skipping e2e/scenario package; set ENABLE_E2E_WASM=true to run")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestScenarios(t *testing.T) {
	registry := NewRegistry(DriveScenarios()...)
	tags := ParseTags(os.Getenv("E2E_SCENARIO_TAGS"))
	adapter, err := devwasm.New(context.Background(), devwasm.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()

	report := registry.Run(context.Background(), adapter, tags)
	if _, err := os.Stdout.WriteString(report.String()); err != nil {
		t.Fatal(err)
	}
	var expected []Expectation
	for _, s := range registry.All() {
		selected := len(tags) == 0
		for _, tag := range tags {
			for _, scenarioTag := range s.Tags {
				if tag == scenarioTag {
					selected = true
				}
			}
		}
		if selected {
			expected = append(expected, Expectation{Scenario: s.Name, Runtime: adapter.Name()})
		}
	}
	if err := report.ValidateExpected(expected); err != nil {
		t.Fatal(err)
	}
}
