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
	b.WriteString("# v")
	b.WriteString(rel.GetVersion())
	if rel.GetDate() != "" {
		b.WriteString("\n\n")
		b.WriteString(rel.GetDate())
	}
	if rel.GetSummaryMarkdown() != "" {
		b.WriteString("\n\n")
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
	b.WriteString("\n\n## ")
	b.WriteString(name)
	for _, entry := range entries {
		b.WriteString("\n\n- ")
		if entry.GetDescriptionMarkdown() != "" {
			b.WriteString(entry.GetDescriptionMarkdown())
			continue
		}
		b.WriteString(entry.GetDescription())
	}
}
