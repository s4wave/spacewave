//go:build !js

package devtool

import "testing"

func TestParsePublishTargetsRejectsEmptySelections(t *testing.T) {
	for _, value := range []string{"", "   ", ",", "release,", "release,   ", "release,,docs"} {
		if _, err := parsePublishTargets(value); err == nil {
			t.Errorf("parsePublishTargets(%q) succeeded", value)
		}
	}
	targets, err := parsePublishTargets(" release , docs ")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0] != "release" || targets[1] != "docs" {
		t.Fatalf("targets = %v", targets)
	}
}
