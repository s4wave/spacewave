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
