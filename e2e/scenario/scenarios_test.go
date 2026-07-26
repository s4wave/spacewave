//go:build e2e_scenario && !js

package scenario

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/s4wave/spacewave/e2e/runtime/devwasm"
)

func TestScenarios(t *testing.T) {
	if os.Getenv("ENABLE_E2E_WASM") != "true" {
		t.Skip("set ENABLE_E2E_WASM=true to run browser scenarios")
	}
	tags := ParseTags(os.Getenv("E2E_SCENARIO_TAGS"))
	adapter, err := devwasm.New(context.Background(), devwasm.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()

	report := Run(context.Background(), adapter, tags)
	fmt.Print(report.String())
	var expected []Expectation
	for _, s := range All() {
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
