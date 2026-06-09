//go:build !js

package bldr

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestAbsolutizeRelativeReplaces(t *testing.T) {
	repoRoot := t.TempDir()
	goMod := []byte(`module github.com/s4wave/spacewave

go 1.25.0

replace github.com/aperturerobotics/bbolt => ../bbolt

replace github.com/aperturerobotics/logrus => github.com/aperturerobotics/logrus v1.9.5-0.20260430110313-9c892333814d
`)

	modFile, err := modfile.Parse("go.mod", goMod, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := absolutizeRelativeReplaces(modFile, repoRoot); err != nil {
		t.Fatal(err)
	}

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

	wantBbolt := filepath.Clean(filepath.Join(repoRoot, "../bbolt"))
	if gotBbolt != wantBbolt {
		t.Fatalf("bbolt replace path = %q, want %q", gotBbolt, wantBbolt)
	}
	if gotLogrus != "github.com/aperturerobotics/logrus" {
		t.Fatalf("module replace path = %q, want module path", gotLogrus)
	}

	formatted, err := modFile.Format()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(formatted); !strings.Contains(got, "github.com/aperturerobotics/bbolt => "+wantBbolt) {
		t.Fatalf("formatted go.mod does not contain absolute bbolt replace:\n%s", got)
	}
}
