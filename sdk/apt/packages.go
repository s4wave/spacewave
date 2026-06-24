package s4wave_apt

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"
)

var aptPackageIndexChecksumAlgorithms = [...]string{"md5", "sha1", "sha256"}

// AptPackagePoolFilename returns the repository pool path for a package.
func AptPackagePoolFilename(pkg *AptPackage) (string, error) {
	if pkg == nil {
		return "", errors.Wrap(ErrInvalidAptPackageIndexMetadata, "package is required")
	}
	name := pkg.GetName()
	version := pkg.GetVersion()
	architecture := pkg.GetArchitecture()
	if err := validateAptPackageName(name); err != nil {
		return "", err
	}
	filenameVersion, err := aptPackageFilenameVersion(version)
	if err != nil {
		return "", err
	}
	if err := validateAptArchitecture(architecture); err != nil {
		return "", err
	}
	first, _ := utf8.DecodeRuneInString(name)
	return path.Join(
		"pool",
		string(first),
		name,
		name+"_"+filenameVersion+"_"+architecture+".deb",
	), nil
}

// GeneratePackagesFile renders published packages in Debian Packages format.
func GeneratePackagesFile(packages []*AptPackage) ([]byte, error) {
	entries := make([]packagesFileEntry, 0, len(packages))
	filenames := make(map[string]struct{}, len(packages))
	for _, pkg := range packages {
		if pkg == nil {
			return nil, errors.Wrap(ErrInvalidAptPackageIndexMetadata, "package is required")
		}
		if err := pkg.GetState().Validate(); err != nil {
			return nil, err
		}
		if pkg.GetState() != AptPackageState_AptPackageState_PUBLISHED {
			continue
		}
		entry, err := newPackagesFileEntry(pkg)
		if err != nil {
			return nil, err
		}
		if _, exists := filenames[entry.filename]; exists {
			return nil, errors.Wrapf(ErrInvalidAptPackageIndexMetadata, "duplicate package filename %q", entry.filename)
		}
		filenames[entry.filename] = struct{}{}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b packagesFileEntry) int {
		return strings.Compare(a.filename, b.filename)
	})

	var buf bytes.Buffer
	for _, entry := range entries {
		writePackagesField(&buf, "Package", entry.pkg.GetName())
		writePackagesField(&buf, "Version", entry.pkg.GetVersion())
		writePackagesField(&buf, "Architecture", entry.pkg.GetArchitecture())
		writePackagesField(&buf, "Filename", entry.filename)
		writePackagesField(&buf, "Size", entry.size)
		writePackagesChecksumField(&buf, "MD5sum", entry.checksums["md5"])
		writePackagesChecksumField(&buf, "SHA1", entry.checksums["sha1"])
		writePackagesChecksumField(&buf, "SHA256", entry.checksums["sha256"])
		writePackagesListField(&buf, "Depends", entry.pkg.GetDepends())
		writePackagesListField(&buf, "Provides", entry.pkg.GetProvides())
		writePackagesListField(&buf, "Conflicts", entry.pkg.GetConflicts())
		writePackagesField(&buf, "Description", entry.pkg.GetDescription())
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// GeneratePackagesGzipFile renders published packages in gzip-compressed Debian Packages format.
func GeneratePackagesGzipFile(packages []*AptPackage) ([]byte, error) {
	data, err := GeneratePackagesFile(packages)
	if err != nil {
		return nil, err
	}
	return CompressPackagesFileGzip(data)
}

// CompressPackagesFileGzip compresses Packages file bytes for Packages.gz.
func CompressPackagesFileGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		if closeErr := gw.Close(); closeErr != nil {
			return nil, errors.Wrapf(err, "compress Packages file; close gzip writer: %v", closeErr)
		}
		return nil, errors.Wrap(err, "compress Packages file")
	}
	if err := gw.Close(); err != nil {
		return nil, errors.Wrap(err, "close Packages gzip writer")
	}
	return buf.Bytes(), nil
}

type packagesFileEntry struct {
	pkg       *AptPackage
	filename  string
	size      string
	checksums map[string]string
}

func newPackagesFileEntry(pkg *AptPackage) (packagesFileEntry, error) {
	if err := pkg.Validate(); err != nil {
		return packagesFileEntry{}, err
	}
	if pkg.GetSize() == 0 {
		return packagesFileEntry{}, errors.Wrap(ErrInvalidAptPackageIndexMetadata, "size is required")
	}
	if strings.TrimSpace(pkg.GetDescription()) == "" {
		return packagesFileEntry{}, errors.Wrap(ErrInvalidAptPackageIndexMetadata, "description is required")
	}
	filename, err := AptPackagePoolFilename(pkg)
	if err != nil {
		return packagesFileEntry{}, err
	}
	checksums := make(map[string]string)
	for _, checksum := range pkg.GetChecksums() {
		algorithm := strings.ToLower(checksum.GetAlgorithm())
		size, ok := aptPackageIndexChecksumHexSize(algorithm)
		if !ok {
			continue
		}
		if _, ok := checksums[algorithm]; ok {
			return packagesFileEntry{}, errors.Wrapf(ErrInvalidAptPackageIndexMetadata, "%s checksum is duplicated", algorithm)
		}
		value := checksum.GetHex()
		if len(value) != size {
			return packagesFileEntry{}, errors.Wrapf(ErrInvalidAptPackageIndexMetadata, "%s checksum has invalid length", algorithm)
		}
		if _, err := hex.DecodeString(value); err != nil {
			return packagesFileEntry{}, errors.Wrapf(ErrInvalidAptPackageIndexMetadata, "%s checksum is invalid hex", algorithm)
		}
		checksums[algorithm] = value
	}
	for _, algorithm := range aptPackageIndexChecksumAlgorithms {
		if checksums[algorithm] == "" {
			return packagesFileEntry{}, errors.Wrapf(ErrInvalidAptPackageIndexMetadata, "%s checksum is required", algorithm)
		}
	}
	return packagesFileEntry{
		pkg:       pkg,
		filename:  filename,
		size:      strconv.FormatUint(pkg.GetSize(), 10),
		checksums: checksums,
	}, nil
}

func aptPackageFilenameVersion(version string) (string, error) {
	var err error
	if epoch, remainder, ok := strings.Cut(version, ":"); ok {
		if epoch == "" || remainder == "" {
			return "", errors.Wrap(ErrInvalidAptPackageIndexMetadata, "version epoch is invalid")
		}
		for _, r := range epoch {
			if r < '0' || r > '9' {
				return "", errors.Wrap(ErrInvalidAptPackageIndexMetadata, "version epoch is invalid")
			}
		}
		if strings.Contains(remainder, ":") {
			return "", errors.Wrap(ErrInvalidAptPackageIndexMetadata, "version contains multiple epoch separators")
		}
		version = remainder
	}
	if err = validateAptPackageVersionBody(version); err != nil {
		return "", err
	}
	return version, nil
}

func aptPackageIndexChecksumHexSize(algorithm string) (int, bool) {
	switch algorithm {
	case "md5":
		return 32, true
	case "sha1":
		return 40, true
	case "sha256":
		return 64, true
	default:
		return 0, false
	}
}

func validateAptPackageName(value string) error {
	if len(value) < 2 {
		return errors.Wrap(ErrInvalidAptPackageIndexMetadata, "name must contain at least two characters")
	}
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case i != 0 && (r == '+' || r == '-' || r == '.'):
		default:
			return errors.Wrap(ErrInvalidAptPackageIndexMetadata, "name contains invalid Debian package name character")
		}
	}
	return nil
}

func validateAptPackageVersionBody(version string) error {
	if version == "" {
		return errors.Wrap(ErrInvalidAptPackageIndexMetadata, "version is required")
	}
	upstream := version
	if idx := strings.LastIndex(version, "-"); idx >= 0 {
		upstream = version[:idx]
		revision := version[idx+1:]
		if upstream == "" || revision == "" {
			return errors.Wrap(ErrInvalidAptPackageIndexMetadata, "version revision is invalid")
		}
		if err := validateAptVersionPart("debian revision", revision, false); err != nil {
			return err
		}
	}
	return validateAptVersionPart("upstream version", upstream, true)
}

func validateAptVersionPart(name, value string, allowHyphen bool) error {
	if value == "" {
		return errors.Wrapf(ErrInvalidAptPackageIndexMetadata, "%s is required", name)
	}
	if name == "upstream version" {
		r, _ := utf8.DecodeRuneInString(value)
		if r < '0' || r > '9' {
			return errors.Wrap(ErrInvalidAptPackageIndexMetadata, "upstream version must start with a digit")
		}
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '+' || r == '~':
		case allowHyphen && r == '-':
		default:
			return errors.Wrapf(ErrInvalidAptPackageIndexMetadata, "%s contains invalid Debian version character", name)
		}
	}
	return nil
}

func validateAptArchitecture(value string) error {
	if value == "" {
		return errors.Wrap(ErrInvalidAptPackageIndexMetadata, "architecture is required")
	}
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' && i != 0 && i != len(value)-1:
		default:
			return errors.Wrap(ErrInvalidAptPackageIndexMetadata, "architecture contains invalid Debian architecture character")
		}
	}
	return nil
}

func writePackagesListField(buf *bytes.Buffer, name string, values []string) {
	if len(values) == 0 {
		return
	}
	writePackagesField(buf, name, strings.Join(values, ", "))
}

func writePackagesChecksumField(buf *bytes.Buffer, name string, value string) {
	if value == "" {
		return
	}
	writePackagesField(buf, name, value)
}

func writePackagesField(buf *bytes.Buffer, name string, value string) {
	lines := strings.Split(value, "\n")
	buf.WriteString(name)
	buf.WriteString(": ")
	buf.WriteString(lines[0])
	buf.WriteByte('\n')
	for _, line := range lines[1:] {
		if line == "" {
			line = "."
		}
		buf.WriteByte(' ')
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
}
