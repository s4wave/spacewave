package changelog

import (
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Validate checks the changelog for the constrained release-note invariants.
func (c *Changelog) Validate() error {
	if c == nil {
		return errors.New("nil changelog")
	}
	if len(c.GetReleases()) == 0 {
		return errors.New("no releases")
	}

	seenVersions := make(map[string]struct{}, len(c.GetReleases()))
	var prevVersion []int
	for i, rel := range c.GetReleases() {
		if rel == nil {
			return errors.Errorf("release %d is nil", i)
		}
		if err := rel.Validate(); err != nil {
			return errors.Wrapf(err, "validate release %d", i)
		}
		version := rel.GetVersion()
		if _, ok := seenVersions[version]; ok {
			return errors.Errorf("duplicate release version %q", version)
		}
		seenVersions[version] = struct{}{}

		currVersion, err := parseReleaseVersion(version)
		if err != nil {
			return errors.Wrapf(err, "validate release %q", version)
		}
		if i > 0 && compareReleaseVersion(prevVersion, currVersion) <= 0 {
			return errors.Errorf(
				"release %q must appear before older release %q",
				version,
				c.GetReleases()[i-1].GetVersion(),
			)
		}
		prevVersion = currVersion
	}

	return nil
}

// Validate checks the release for required user-facing fields.
func (r *Release) Validate() error {
	if r == nil {
		return errors.New("nil release")
	}
	if _, err := parseReleaseVersion(r.GetVersion()); err != nil {
		return err
	}
	if _, err := time.Parse("2006-01-02", r.GetDate()); err != nil {
		return errors.Wrap(err, "invalid release date")
	}
	if strings.TrimSpace(r.GetSummary()) == "" {
		return errors.New("missing release summary")
	}

	hasEntry := false
	for _, entries := range [][]*ChangeEntry{
		r.GetFeatures(),
		r.GetFixes(),
		r.GetImprovements(),
		r.GetSecurity(),
	} {
		for j, entry := range entries {
			hasEntry = true
			if err := entry.Validate(); err != nil {
				return errors.Wrapf(err, "validate change entry %d", j)
			}
		}
	}
	if !hasEntry {
		return errors.New("release must contain at least one change entry")
	}

	return nil
}

// Validate checks the user-facing change entry fields.
func (e *ChangeEntry) Validate() error {
	if e == nil {
		return errors.New("nil change entry")
	}
	if strings.TrimSpace(e.GetDescription()) == "" {
		return errors.New("missing change description")
	}
	return nil
}

func parseReleaseVersion(version string) ([]int, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return nil, errors.Errorf("invalid semver %q", version)
	}
	out := make([]int, 3)
	for i, part := range parts {
		val, err := strconv.Atoi(part)
		if err != nil || val < 0 {
			return nil, errors.Errorf("invalid semver %q", version)
		}
		out[i] = val
	}
	return out, nil
}

func compareReleaseVersion(a []int, b []int) int {
	for i := range min(len(a), len(b)) {
		if a[i] > b[i] {
			return 1
		}
		if a[i] < b[i] {
			return -1
		}
	}
	switch {
	case len(a) > len(b):
		return 1
	case len(a) < len(b):
		return -1
	default:
		return 0
	}
}
