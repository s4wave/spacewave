//go:build !js

package bldr_project_starlark

import (
	"strings"
	"testing"
	"testing/fstest"
)

// varProject reads a deployment input in the root file and in a loaded module,
// so both must see the same value.
var varProject = fstest.MapFS{
	"bldr.star": {Data: []byte(`
load("lib/deploy.star", "DEPLOYMENT")
project(id=var("deployment", "production") + "-" + DEPLOYMENT)
`)},
	"lib/deploy.star": {Data: []byte(`DEPLOYMENT = var("deployment", "production")`)},
}

// TestEvaluateFSVarDefault uses the default value when no input was passed.
func TestEvaluateFSVarDefault(t *testing.T) {
	// Evaluate with no inputs.
	result, err := EvaluateFS(t.Context(), varProject, "bldr.star")
	if err != nil {
		t.Fatal(err)
	}

	// Both files must see the default value.
	if got := result.Config.GetId(); got != "production-production" {
		t.Fatalf("expected the default in both files, got %q", got)
	}
}

// TestEvaluateFSVarOverride returns the passed input to the root file and to a
// loaded module.
func TestEvaluateFSVarOverride(t *testing.T) {
	// Evaluate with the deployment input set.
	result, err := EvaluateFS(t.Context(), varProject, "bldr.star", WithVars(map[string]string{"deployment": "staging"}))
	if err != nil {
		t.Fatal(err)
	}

	// Both files must see the input.
	if got := result.Config.GetId(); got != "staging-staging" {
		t.Fatalf("expected the input in both files, got %q", got)
	}
}

// TestEvaluateFSVarIgnoresEnvironment never reads the process environment,
// even for a name that matches an environment variable.
func TestEvaluateFSVarIgnoresEnvironment(t *testing.T) {
	// Set a matching environment variable, then pass no inputs.
	t.Setenv("deployment", "staging")
	result, err := EvaluateFS(t.Context(), varProject, "bldr.star")
	if err != nil {
		t.Fatal(err)
	}

	// The environment must not reach the script.
	if got := result.Config.GetId(); got != "production-production" {
		t.Fatalf("expected the default, got %q", got)
	}
}

// TestEvaluateFSVarWithoutDefault fails when an input is neither passed nor
// defaulted.
func TestEvaluateFSVarWithoutDefault(t *testing.T) {
	// Read an input that has no default.
	fsys := fstest.MapFS{"bldr.star": {Data: []byte(`project(id=var("deployment"))`)}}

	// Without the input the evaluation fails.
	_, err := EvaluateFS(t.Context(), fsys, "bldr.star")
	if err == nil || !strings.Contains(err.Error(), `input "deployment" was not passed`) {
		t.Fatalf("expected a missing input error, got %v", err)
	}

	// With the input it succeeds.
	result, err := EvaluateFS(t.Context(), fsys, "bldr.star", WithVars(map[string]string{"deployment": "staging"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Config.GetId(); got != "staging" {
		t.Fatalf("expected the input, got %q", got)
	}
}

// TestEvaluateFSVarRejectsEmptyName refuses a call that selects no input.
func TestEvaluateFSVarRejectsEmptyName(t *testing.T) {
	// Select no input in the call.
	fsys := fstest.MapFS{"bldr.star": {Data: []byte(`project(id=var("", "x"))`)}}

	// The evaluation must fail.
	if _, err := EvaluateFS(t.Context(), fsys, "bldr.star"); err == nil {
		t.Fatal("expected an error for an empty name")
	}
}

// TestEvaluateFSBoundedVar gives the child the same inputs the in-process
// evaluation sees, and an empty value is passed, not defaulted.
func TestEvaluateFSBoundedVar(t *testing.T) {
	// Evaluate with a value, with an empty value, and with no input.
	for _, tc := range []struct {
		name string
		vars map[string]string
		want string
	}{
		{"override", map[string]string{"deployment": "staging"}, "staging-staging"},
		{"empty", map[string]string{"deployment": ""}, "-"},
		{"default", nil, "production-production"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The bounded child must not read this process's environment.
			t.Setenv("deployment", "leaked")
			result, err := EvaluateFSBounded(t.Context(), varProject, "bldr.star", 256<<20, WithVars(tc.vars))
			if err != nil {
				t.Fatal(err)
			}
			if got := result.Config.GetId(); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// TestEvaluateFSBoundedVarMissing reports a missing input from the child.
func TestEvaluateFSBoundedVarMissing(t *testing.T) {
	// Read an input that has no default and pass none.
	fsys := fstest.MapFS{"bldr.star": {Data: []byte(`project(id=var("deployment"))`)}}

	// The child's error must reach the caller.
	_, err := EvaluateFSBounded(t.Context(), fsys, "bldr.star", 256<<20)
	if err == nil || !strings.Contains(err.Error(), `input "deployment" was not passed`) {
		t.Fatalf("expected a missing input error, got %v", err)
	}
}
