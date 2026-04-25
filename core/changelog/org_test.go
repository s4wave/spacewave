package changelog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseOrgChangelog(t *testing.T) {
	cl, err := ParseOrgChangelog([]byte(`#+TITLE: Spacewave Changelog

* v0.1.0 Alpha Launch
<2026-03-20 Fri>

Launch release of Spacewave with encrypted local-first storage and collaborative
Spaces.

** Features
- Drive: encrypted file storage and sync with drag-and-drop upload, projected
  downloads, and zip export.
- Canvas: an infinite 2D workspace for visual thinking.

** Fixes
- Better popout and new-tab anchoring across the shell.

** Improvements
- Session settings use collapsible sections and a clearer account-centered
  layout.

** Security
- Added [[https://github.com/s4wave/spacewave/pull/123][review links]] to
  the release copy.
`))
	if err != nil {
		t.Fatalf("ParseOrgChangelog() error = %v", err)
	}

	if len(cl.GetReleases()) != 1 {
		t.Fatalf("expected 1 release, got %d", len(cl.GetReleases()))
	}
	rel := cl.GetReleases()[0]
	if rel.GetVersion() != "0.1.0" {
		t.Fatalf("expected version 0.1.0, got %q", rel.GetVersion())
	}
	if rel.GetDate() != "2026-03-20" {
		t.Fatalf("expected date 2026-03-20, got %q", rel.GetDate())
	}
	if rel.GetSummary() != "Launch release of Spacewave with encrypted local-first storage and collaborative Spaces." {
		t.Fatalf("unexpected summary %q", rel.GetSummary())
	}
	if rel.GetSummaryMarkdown() != "Launch release of Spacewave with encrypted local-first storage and collaborative Spaces." {
		t.Fatalf("unexpected summary markdown %q", rel.GetSummaryMarkdown())
	}
	if len(rel.GetFeatures()) != 2 {
		t.Fatalf("expected 2 features, got %d", len(rel.GetFeatures()))
	}
	if rel.GetFeatures()[0].GetDescription() != "Drive: encrypted file storage and sync with drag-and-drop upload, projected downloads, and zip export." {
		t.Fatalf("unexpected first feature %q", rel.GetFeatures()[0].GetDescription())
	}
	if rel.GetFeatures()[0].GetDescriptionMarkdown() != "Drive: encrypted file storage and sync with drag-and-drop upload, projected downloads, and zip export." {
		t.Fatalf(
			"unexpected first feature markdown %q",
			rel.GetFeatures()[0].GetDescriptionMarkdown(),
		)
	}
	if len(rel.GetSecurity()) != 1 {
		t.Fatalf("expected 1 security entry, got %d", len(rel.GetSecurity()))
	}
	if rel.GetSecurity()[0].GetDescription() != "Added review links to the release copy." {
		t.Fatalf("unexpected security entry %q", rel.GetSecurity()[0].GetDescription())
	}
	if rel.GetSecurity()[0].GetDescriptionMarkdown() != "Added [review links](https://github.com/s4wave/spacewave/pull/123) to the release copy." {
		t.Fatalf(
			"unexpected security entry markdown %q",
			rel.GetSecurity()[0].GetDescriptionMarkdown(),
		)
	}
}

func TestParseOrgChangelogCurrentFileMatchesArtifacts(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	rootDir := filepath.Clean(filepath.Join(cwd, "..", ".."))

	orgData, err := os.ReadFile(filepath.Join(rootDir, "CHANGELOG.org"))
	if err != nil {
		t.Fatalf("os.ReadFile(CHANGELOG.org) error = %v", err)
	}
	got, err := ParseOrgChangelog(orgData)
	if err != nil {
		t.Fatalf("ParseOrgChangelog() error = %v", err)
	}

	binData, err := os.ReadFile(filepath.Join(rootDir, "core", "changelog", "changelog.bin"))
	if err != nil {
		t.Fatalf("os.ReadFile(changelog.bin) error = %v", err)
	}
	gotFromBin := &Changelog{}
	if err := gotFromBin.UnmarshalVT(binData); err != nil {
		t.Fatalf("UnmarshalVT(changelog.bin) error = %v", err)
	}
	if !reflect.DeepEqual(got, gotFromBin) {
		t.Fatalf("parsed org changelog did not match changelog.bin")
	}

	gotEmbedded, err := GetChangelog()
	if err != nil {
		t.Fatalf("GetChangelog() error = %v", err)
	}
	if !reflect.DeepEqual(got, gotEmbedded) {
		t.Fatalf("parsed org changelog did not match embedded changelog")
	}

	marshaledData, err := got.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT() error = %v", err)
	}
	roundTrip := &Changelog{}
	if err := roundTrip.UnmarshalVT(marshaledData); err != nil {
		t.Fatalf("UnmarshalVT() error = %v", err)
	}
	if !reflect.DeepEqual(got, roundTrip) {
		t.Fatalf("protobuf round trip changed changelog")
	}
}

func TestParseOrgChangelogValidation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "duplicate versions",
			input: `* v0.1.0 First
<2026-03-20 Fri>

First summary.

** Features
- First feature.

* v0.1.0 Duplicate
<2026-03-19 Thu>

Second summary.

** Fixes
- Second fix.
`,
			wantErr: `duplicate release version "0.1.0"`,
		},
		{
			name: "ascending order",
			input: `* v0.1.0 First
<2026-03-20 Fri>

First summary.

** Features
- First feature.

* v0.2.0 Newer
<2026-03-21 Sat>

Second summary.

** Fixes
- Second fix.
`,
			wantErr: `must appear before older release`,
		},
		{
			name: "missing summary",
			input: `* v0.1.0 First
<2026-03-20 Fri>

** Features
- First feature.
`,
			wantErr: "missing summary for 0.1.0",
		},
		{
			name: "no entries",
			input: `* v0.1.0 First
<2026-03-20 Fri>

Summary.

** Features
`,
			wantErr: "release must contain at least one change entry",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseOrgChangelog([]byte(tt.input))
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
