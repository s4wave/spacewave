package s4wave_apt

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/pkg/errors"
)

const (
	debArMagic      = "!<arch>\n"
	debArHeaderSize = 60
)

// ParseDebPackageFile parses apt package metadata from a local .deb file.
func ParseDebPackageFile(debPath string) (*AptPackage, error) {
	f, err := os.Open(debPath)
	if err != nil {
		return nil, errors.Wrap(err, "open deb package")
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "stat deb package")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("deb package is not a regular file")
	}
	return ParseDebPackageReader(f, info.Size())
}

// ParseDebPackage parses apt package metadata from Debian binary package data.
func ParseDebPackage(data []byte) (*AptPackage, error) {
	return ParseDebPackageReader(bytes.NewReader(data), int64(len(data)))
}

// ParseDebPackageReader parses apt package metadata from a Debian binary package.
func ParseDebPackageReader(r io.ReaderAt, size int64) (*AptPackage, error) {
	if size < int64(len(debArMagic)) {
		return nil, errors.Wrap(ErrInvalidDebPackage, "short ar header")
	}
	magic := make([]byte, len(debArMagic))
	if _, err := r.ReadAt(magic, 0); err != nil {
		return nil, errors.Wrap(err, "read ar header")
	}
	if string(magic) != debArMagic {
		return nil, errors.Wrap(ErrInvalidDebPackage, "missing ar header")
	}

	var (
		sawDebianBinary bool
		sawDataArchive  bool
		controlPackage  *AptPackage
	)
	off := int64(len(debArMagic))
	for off < size {
		member, err := readDebArMember(r, off, size)
		if err != nil {
			return nil, err
		}
		switch member.name {
		case "debian-binary":
			if off != int64(len(debArMagic)) {
				return nil, errors.Wrap(ErrInvalidDebPackage, "debian-binary is not first ar member")
			}
			if err := validateDebianBinaryMember(r, member); err != nil {
				return nil, err
			}
			sawDebianBinary = true
		default:
			if strings.HasPrefix(member.name, "control.tar") {
				if !sawDebianBinary {
					return nil, errors.Wrap(ErrInvalidDebPackage, "control archive before debian-binary")
				}
				if controlPackage != nil {
					return nil, errors.Wrap(ErrInvalidDebPackage, "duplicate control archive")
				}
				controlPackage, err = parseDebControlArchive(r, member, uint64(size))
				if err != nil {
					return nil, err
				}
			} else if strings.HasPrefix(member.name, "data.tar") {
				if controlPackage == nil {
					return nil, errors.Wrap(ErrInvalidDebPackage, "data archive before control archive")
				}
				sawDataArchive = true
			}
		}
		off = member.nextOffset
	}
	if !sawDebianBinary {
		return nil, errors.Wrap(ErrInvalidDebPackage, "debian-binary member not found")
	}
	if controlPackage == nil {
		return nil, errors.Wrap(ErrInvalidDebPackage, "control archive not found")
	}
	if !sawDataArchive {
		return nil, errors.Wrap(ErrInvalidDebPackage, "data archive not found")
	}
	return controlPackage, nil
}

type debArMember struct {
	name       string
	offset     int64
	size       int64
	nextOffset int64
}

func readDebArMember(r io.ReaderAt, off int64, debSize int64) (*debArMember, error) {
	if off+debArHeaderSize > debSize {
		return nil, errors.Wrap(ErrInvalidDebPackage, "short ar member header")
	}
	header := make([]byte, debArHeaderSize)
	if _, err := r.ReadAt(header, off); err != nil {
		return nil, errors.Wrap(err, "read ar member header")
	}
	if string(header[58:60]) != "`\n" {
		return nil, errors.Wrap(ErrInvalidDebPackage, "invalid ar member trailer")
	}
	name := strings.TrimSpace(string(header[0:16]))
	name = strings.TrimSuffix(name, "/")
	memberSize, err := strconv.ParseInt(strings.TrimSpace(string(header[48:58])), 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "parse ar member size")
	}
	if memberSize < 0 {
		return nil, errors.Wrap(ErrInvalidDebPackage, "negative ar member size")
	}
	dataOffset := off + debArHeaderSize
	dataEnd := dataOffset + memberSize
	if dataEnd > debSize {
		return nil, errors.Wrap(ErrInvalidDebPackage, "ar member exceeds package size")
	}
	nextOffset := dataEnd
	if nextOffset%2 != 0 {
		nextOffset++
	}
	return &debArMember{
		name:       name,
		offset:     dataOffset,
		size:       memberSize,
		nextOffset: nextOffset,
	}, nil
}

func validateDebianBinaryMember(r io.ReaderAt, member *debArMember) error {
	if member.size > 64 {
		return errors.Wrap(ErrInvalidDebPackage, "debian-binary member too large")
	}
	data := make([]byte, member.size)
	if _, err := r.ReadAt(data, member.offset); err != nil {
		return errors.Wrap(err, "read debian-binary member")
	}
	if strings.TrimSpace(string(data)) != "2.0" {
		return errors.Wrap(ErrInvalidDebPackage, "unsupported debian-binary version")
	}
	return nil
}

func parseDebControlArchive(r io.ReaderAt, member *debArMember, debSize uint64) (*AptPackage, error) {
	rc, err := openDebControlArchive(member.name, io.NewSectionReader(r, member.offset, member.size))
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil, errors.Wrap(ErrInvalidDebPackage, "control file not found")
		}
		if err != nil {
			return nil, errors.Wrap(err, "read control archive")
		}
		if header.Typeflag == tar.TypeDir || path.Clean(header.Name) != "control" {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, errors.Wrap(err, "read control file")
		}
		return parseDebControlFile(data, debSize)
	}
}

func openDebControlArchive(name string, r io.Reader) (io.ReadCloser, error) {
	switch {
	case strings.HasSuffix(name, ".tar"):
		return io.NopCloser(r), nil
	case strings.HasSuffix(name, ".tar.gz"):
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, errors.Wrap(err, "open gzip control archive")
		}
		return gr, nil
	case strings.HasSuffix(name, ".tar.zst") || strings.HasSuffix(name, ".tar.zstd"):
		zr, err := zstd.NewReader(r)
		if err != nil {
			return nil, errors.Wrap(err, "open zstd control archive")
		}
		return zr.IOReadCloser(), nil
	default:
		return nil, errors.Wrap(ErrUnsupportedDebControlCompression, name)
	}
}

func parseDebControlFile(data []byte, debSize uint64) (*AptPackage, error) {
	fields, err := parseDebControlFields(string(data))
	if err != nil {
		return nil, err
	}
	name, err := requireDebControlField(fields, "package")
	if err != nil {
		return nil, err
	}
	version, err := requireDebControlField(fields, "version")
	if err != nil {
		return nil, err
	}
	architecture, err := requireDebControlField(fields, "architecture")
	if err != nil {
		return nil, err
	}
	description, err := requireDebControlField(fields, "description")
	if err != nil {
		return nil, err
	}
	return &AptPackage{
		State:        AptPackageState_AptPackageState_IMPORTING,
		Name:         name,
		Version:      version,
		Architecture: architecture,
		Depends:      splitDebControlList(fields["depends"]),
		Provides:     splitDebControlList(fields["provides"]),
		Conflicts:    splitDebControlList(fields["conflicts"]),
		Description:  description,
		Size:         debSize,
	}, nil
}

func parseDebControlFields(control string) (map[string]string, error) {
	control = strings.ReplaceAll(control, "\r\n", "\n")
	fields := make(map[string]string)
	var current string
	for raw := range strings.SplitSeq(control, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if line == "" {
			break
		}
		if line[0] == ' ' || line[0] == '\t' {
			if current == "" {
				return nil, errors.Wrap(ErrInvalidDebPackage, "control continuation without field")
			}
			fields[current] = appendDebControlContinuation(fields[current], line)
			continue
		}
		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			return nil, errors.Wrap(ErrInvalidDebPackage, "invalid control field")
		}
		current = strings.ToLower(line[:idx])
		if _, ok := fields[current]; ok {
			return nil, errors.Wrapf(ErrInvalidDebPackage, "duplicate control field %s", current)
		}
		fields[current] = strings.TrimSpace(line[idx+1:])
	}
	return fields, nil
}

func appendDebControlContinuation(value string, line string) string {
	line = strings.TrimLeft(line, " \t")
	if line == "." {
		line = ""
	}
	if value == "" {
		return line
	}
	return value + "\n" + line
}

func requireDebControlField(fields map[string]string, name string) (string, error) {
	value := strings.TrimSpace(fields[name])
	if value == "" {
		return "", errors.Wrapf(ErrInvalidDebPackage, "missing control field %s", name)
	}
	return value, nil
}

func splitDebControlList(value string) []string {
	value = strings.ReplaceAll(value, "\n", " ")
	var out []string
	for item := range strings.SplitSeq(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
