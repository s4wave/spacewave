package changelog

import (
	"strings"
	"testing"
)

func TestRenderReleaseMarkdownIncludesDownloadLinks(t *testing.T) {
	cl := &Changelog{
		Releases: []*Release{{
			Version:         "0.51.6",
			SummaryMarkdown: "Spacewave test release.",
			Fixes: []*ChangeEntry{{
				DescriptionMarkdown: "Fixed release notes.",
			}},
		}},
	}

	got, err := RenderReleaseMarkdown(cl, "v0.51.6")
	if err != nil {
		t.Fatalf("RenderReleaseMarkdown() error = %v", err)
	}
	for _, want := range []string{
		"## Downloads",
		"[MacOS (arm64)](https://github.com/s4wave/spacewave/releases/download/v0.51.6/spacewave-macos-arm64.dmg)",
		"[Windows (amd64)](https://github.com/s4wave/spacewave/releases/download/v0.51.6/spacewave-windows-amd64.msix)",
		"[Linux (amd64)](https://github.com/s4wave/spacewave/releases/download/v0.51.6/spacewave-linux-amd64.AppImage)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered notes missing %q:\n%s", want, got)
		}
	}
}
