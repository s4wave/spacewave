package changelog

import (
	"strings"

	"github.com/pkg/errors"
)

// ParseOrgChangelog parses the constrained CHANGELOG.org subset.
func ParseOrgChangelog(data []byte) (*Changelog, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	cl := &Changelog{}
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#+") {
			i++
			continue
		}
		if !strings.HasPrefix(line, "* ") {
			i++
			continue
		}

		rel, next, err := parseOrgRelease(lines, i)
		if err != nil {
			return nil, err
		}
		cl.Releases = append(cl.Releases, rel)
		i = next
	}
	if len(cl.GetReleases()) == 0 {
		return nil, errors.New("no releases found in CHANGELOG.org")
	}
	if err := cl.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate CHANGELOG.org")
	}
	return cl, nil
}

func parseOrgRelease(lines []string, i int) (*Release, int, error) {
	line := strings.TrimSpace(lines[i])
	if !strings.HasPrefix(line, "* ") {
		return nil, 0, errors.Errorf("expected release heading at line %d", i+1)
	}
	fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "* ")))
	if len(fields) == 0 {
		return nil, 0, errors.Errorf("missing release version at line %d", i+1)
	}
	version := strings.TrimPrefix(fields[0], "v")
	if version == "" || version == fields[0] {
		return nil, 0, errors.Errorf("invalid release version at line %d", i+1)
	}

	i++
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) {
		return nil, 0, errors.Errorf("missing release date for %s", version)
	}
	date, err := parseOrgDate(lines[i])
	if err != nil {
		return nil, 0, errors.Wrapf(err, "parse release date for %s", version)
	}
	i++

	summary, next := collectOrgParagraph(lines, i)
	if summary == "" {
		return nil, 0, errors.Errorf("missing summary for %s", version)
	}
	rel := &Release{
		Version:         version,
		Date:            date,
		Summary:         renderOrgInlinePlain(summary),
		SummaryMarkdown: renderOrgInlineMarkdown(summary),
	}

	for i = next; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}
		if strings.HasPrefix(line, "* ") {
			break
		}
		if !strings.HasPrefix(line, "** ") {
			return nil, 0, errors.Errorf(
				"unexpected release content for %s at line %d",
				version,
				i+1,
			)
		}

		section := strings.TrimSpace(strings.TrimPrefix(line, "** "))
		entries, next, err := collectOrgEntries(lines, i+1)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "parse %s section for %s", section, version)
		}

		switch section {
		case "Features":
			rel.Features = entries
		case "Fixes":
			rel.Fixes = entries
		case "Improvements":
			rel.Improvements = entries
		case "Security":
			rel.Security = entries
		default:
			return nil, 0, errors.Errorf(
				"unsupported section %q for %s at line %d",
				section,
				version,
				i+1,
			)
		}
		i = next
	}

	return rel, i, nil
}

func parseOrgDate(line string) (string, error) {
	line = strings.TrimSpace(line)
	if len(line) < len("<2006-01-02") || line[0] != '<' {
		return "", errors.New("date line must start with an org timestamp")
	}
	date := line[1:11]
	if len(line) < 12 || (line[11] != ' ' && line[11] != '>') {
		return "", errors.New("org timestamp must start with YYYY-MM-DD")
	}
	return date, nil
}

func collectOrgParagraph(lines []string, i int) (string, int) {
	var parts []string
	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			if len(parts) == 0 {
				continue
			}
			i++
			break
		}
		if strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "** ") {
			break
		}
		parts = append(parts, line)
	}
	return strings.Join(parts, " "), i
}

func collectOrgEntries(lines []string, i int) ([]*ChangeEntry, int, error) {
	var entries []*ChangeEntry
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}
		if strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "** ") {
			break
		}
		if !strings.HasPrefix(line, "- ") {
			return nil, 0, errors.Errorf("expected bullet at line %d", i+1)
		}

		var parts []string
		parts = append(parts, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
		i++
		for ; i < len(lines); i++ {
			line = strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "- ") ||
				strings.HasPrefix(line, "* ") ||
				strings.HasPrefix(line, "** ") {
				break
			}
			parts = append(parts, line)
		}
		description := strings.Join(parts, " ")
		entries = append(entries, &ChangeEntry{
			Description:         renderOrgInlinePlain(description),
			DescriptionMarkdown: renderOrgInlineMarkdown(description),
		})
	}
	return entries, i, nil
}

func renderOrgInlinePlain(s string) string {
	return renderOrgInline(s, func(url, label string) string {
		if label != "" {
			return label
		}
		return url
	})
}

func renderOrgInlineMarkdown(s string) string {
	return renderOrgInline(s, func(url, label string) string {
		if label == "" {
			label = url
		}
		return "[" + label + "](" + url + ")"
	})
}

func renderOrgInline(
	s string,
	renderLink func(url string, label string) string,
) string {
	var b strings.Builder
	for len(s) > 0 {
		start := strings.Index(s, "[[")
		if start < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:start])
		s = s[start+2:]

		end := strings.Index(s, "]]")
		if end < 0 {
			b.WriteString("[[")
			b.WriteString(s)
			break
		}
		link := s[:end]
		s = s[end+2:]

		if before, after, ok := strings.Cut(link, "]["); ok {
			b.WriteString(renderLink(before, after))
			continue
		}
		b.WriteString(renderLink(link, ""))
	}
	return b.String()
}
