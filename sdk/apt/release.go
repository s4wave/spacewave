package s4wave_apt

import (
	"bytes"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// GenerateReleaseFile renders Debian Release metadata for generated index files.
func GenerateReleaseFile(repo *AptRepository, files map[string][]byte, date time.Time) ([]byte, error) {
	if repo == nil {
		return nil, errors.Wrap(ErrInvalidAptPackageIndexMetadata, "repository is required")
	}
	if err := repo.Validate(); err != nil {
		return nil, err
	}
	if date.IsZero() {
		return nil, errors.Wrap(ErrInvalidAptPackageIndexMetadata, "date is required")
	}
	distribution, components, architectures, err := aptReleaseMetadata(repo)
	if err != nil {
		return nil, err
	}
	entries, err := newReleaseFileEntries(files)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	writePackagesField(&buf, "Suite", distribution)
	writePackagesField(&buf, "Codename", distribution)
	writePackagesField(&buf, "Date", date.UTC().Format(time.RFC1123))
	writePackagesField(&buf, "Architectures", strings.Join(architectures, " "))
	writePackagesField(&buf, "Components", strings.Join(components, " "))
	writeReleaseChecksumSection(&buf, "MD5Sum", entries, func(c aptChecksumSet) string {
		return c.md5
	})
	writeReleaseChecksumSection(&buf, "SHA1", entries, func(c aptChecksumSet) string {
		return c.sha1
	})
	writeReleaseChecksumSection(&buf, "SHA256", entries, func(c aptChecksumSet) string {
		return c.sha256
	})
	return buf.Bytes(), nil
}

func aptReleaseMetadata(repo *AptRepository) (string, []string, []string, error) {
	distribution := repo.GetDistribution()
	if err := validateAptReleaseToken("distribution", distribution); err != nil {
		return "", nil, nil, err
	}
	components := slices.Clone(repo.GetComponents())
	for _, component := range components {
		if err := validateAptReleaseToken("component", component); err != nil {
			return "", nil, nil, err
		}
	}
	architectures := slices.Clone(repo.GetArchitectures())
	for _, architecture := range architectures {
		if err := validateAptArchitecture(architecture); err != nil {
			return "", nil, nil, err
		}
	}
	return distribution, components, architectures, nil
}

type releaseFileEntry struct {
	path      string
	size      string
	checksums aptChecksumSet
}

func newReleaseFileEntries(files map[string][]byte) ([]releaseFileEntry, error) {
	if len(files) == 0 {
		return nil, errors.Wrap(ErrInvalidAptPackageIndexMetadata, "release files are required")
	}
	entries := make([]releaseFileEntry, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for filePath, data := range files {
		clean, err := cleanReleaseFilePath(filePath)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[clean]; exists {
			return nil, errors.Wrapf(ErrInvalidAptPackageIndexMetadata, "duplicate release file path %q", clean)
		}
		seen[clean] = struct{}{}
		entries = append(entries, releaseFileEntry{
			path:      clean,
			size:      strconv.FormatUint(uint64(len(data)), 10),
			checksums: newAptChecksumSet(data),
		})
	}
	slices.SortFunc(entries, func(a, b releaseFileEntry) int {
		return strings.Compare(a.path, b.path)
	})
	return entries, nil
}

func cleanReleaseFilePath(filePath string) (string, error) {
	if filePath == "" {
		return "", errors.Wrap(ErrInvalidAptPackageIndexMetadata, "release file path is required")
	}
	clean := path.Clean(filePath)
	if clean == "." || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.Wrap(ErrInvalidAptPackageIndexMetadata, "release file path escapes index root")
	}
	for _, r := range clean {
		if r <= ' ' || r > '~' {
			return "", errors.Wrap(ErrInvalidAptPackageIndexMetadata, "release file path contains invalid character")
		}
	}
	return clean, nil
}

func validateAptReleaseToken(name, value string) error {
	if value == "" {
		return errors.Wrapf(ErrInvalidAptPackageIndexMetadata, "%s is required", name)
	}
	if value == "." || value == ".." {
		return errors.Wrapf(ErrInvalidAptPackageIndexMetadata, "%s is a path segment", name)
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '+' || r == '-' || r == '.' || r == '_' || r == '~':
		default:
			return errors.Wrapf(ErrInvalidAptPackageIndexMetadata, "%s contains invalid release metadata character", name)
		}
	}
	return nil
}

func writeReleaseChecksumSection(
	buf *bytes.Buffer,
	name string,
	entries []releaseFileEntry,
	checksum func(aptChecksumSet) string,
) {
	buf.WriteString(name)
	buf.WriteString(":\n")
	for _, entry := range entries {
		buf.WriteByte(' ')
		buf.WriteString(checksum(entry.checksums))
		buf.WriteByte(' ')
		buf.WriteString(entry.size)
		buf.WriteByte(' ')
		buf.WriteString(entry.path)
		buf.WriteByte('\n')
	}
}
