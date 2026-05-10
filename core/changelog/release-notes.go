package changelog

import (
	"strings"

	"github.com/pkg/errors"
)

// RenderReleaseMarkdown renders one release as GitHub release-note markdown.
func RenderReleaseMarkdown(cl *Changelog, version string) (string, error) {
	if cl == nil {
		return "", errors.New("nil changelog")
	}
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	for _, rel := range cl.GetReleases() {
		if rel.GetVersion() == version {
			return renderReleaseMarkdown(rel), nil
		}
	}
	return "", errors.New("release not found: " + version)
}

func renderReleaseMarkdown(rel *Release) string {
	var b strings.Builder
	if rel.GetSummaryMarkdown() != "" {
		b.WriteString(rel.GetSummaryMarkdown())
	}
	writeChangeSection(&b, "Features", rel.GetFeatures())
	writeChangeSection(&b, "Fixes", rel.GetFixes())
	writeChangeSection(&b, "Improvements", rel.GetImprovements())
	writeChangeSection(&b, "Security", rel.GetSecurity())
	writeDownloadSection(&b, rel.GetVersion())
	b.WriteString("\n")
	return b.String()
}

func writeChangeSection(b *strings.Builder, name string, entries []*ChangeEntry) {
	if len(entries) == 0 {
		return
	}
	if b.Len() != 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("## ")
	b.WriteString(name)
	for _, entry := range entries {
		b.WriteString("\n- ")
		if entry.GetDescriptionMarkdown() != "" {
			b.WriteString(entry.GetDescriptionMarkdown())
			continue
		}
		b.WriteString(entry.GetDescription())
	}
}

func writeDownloadSection(b *strings.Builder, version string) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version == "" {
		return
	}
	if b.Len() != 0 {
		b.WriteString("\n\n")
	}
	tag := "v" + version
	base := "https://github.com/s4wave/spacewave/releases/download/" + tag + "/"
	b.WriteString("## Downloads\n")
	b.WriteString("[MacOS (arm64)](")
	b.WriteString(base)
	b.WriteString("spacewave-macos-arm64.dmg) | [Windows (amd64)](")
	b.WriteString(base)
	b.WriteString("spacewave-windows-amd64.msix) | [Linux (amd64)](")
	b.WriteString(base)
	b.WriteString("spacewave-linux-amd64.AppImage)")
}
