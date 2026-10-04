//go:build !js

package bldr

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestAbsolutizeRelativeReplaces(t *testing.T) {
	// Prepare a temporary module with relative and versioned replacements.
	repoRoot := t.TempDir()
	goMod := []byte(`module github.com/s4wave/spacewave

go 1.25.0

replace github.com/aperturerobotics/bbolt => ../bbolt

replace github.com/aperturerobotics/logrus => github.com/aperturerobotics/logrus v1.9.5-0.20260430110313-9c892333814d
`)

	// Parse the replacement fixture into the module representation.
	modFile, err := modfile.Parse("go.mod", goMod, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve local replacement paths against the temporary repository root.
	if err := absolutizeRelativeReplaces(modFile, repoRoot); err != nil {
		t.Fatal(err)
	}

	// Collect replacement paths by the original module identity.
	var gotBbolt string
	var gotLogrus string
	for _, replace := range modFile.Replace {
		switch replace.Old.Path {
		case "github.com/aperturerobotics/bbolt":
			gotBbolt = replace.New.Path
		case "github.com/aperturerobotics/logrus":
			gotLogrus = replace.New.Path
		}
	}

	// Verify local paths become absolute while versioned paths stay module paths.
	wantBbolt := filepath.Clean(filepath.Join(repoRoot, "../bbolt"))
	if gotBbolt != wantBbolt {
		t.Fatalf("bbolt replace path = %q, want %q", gotBbolt, wantBbolt)
	}
	if gotLogrus != "github.com/aperturerobotics/logrus" {
		t.Fatalf("module replace path = %q, want module path", gotLogrus)
	}

	// Format the rewritten module and verify its absolute local replacement.
	formatted, err := modFile.Format()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(formatted); !strings.Contains(got, "github.com/aperturerobotics/bbolt => "+wantBbolt) {
		t.Fatalf("formatted go.mod does not contain absolute bbolt replace:\n%s", got)
	}
}
