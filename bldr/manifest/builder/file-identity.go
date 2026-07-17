package bldr_manifest_builder

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"

	"github.com/pkg/errors"
)

// CaptureFileIdentity captures the content identity for one file path.
//
// The identity records size, modification time, and SHA-256 digest so a later
// validation can take a cheap size+modtime fast path and fall back to a content
// comparison when only the modification time changed.
func CaptureFileIdentity(filePath string) (*InputManifest_FileIdentity, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if fileInfo.IsDir() {
		return nil, errors.Errorf("path is a directory: %s", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return nil, err
	}
	if fileInfo.Size() < 0 {
		return nil, errors.Errorf("negative file size: %d", fileInfo.Size())
	}
	return &InputManifest_FileIdentity{
		// #nosec G115 -- fileInfo.Size() is validated as non-negative immediately above.
		SizeBytes:       uint64(fileInfo.Size()),
		ModTimeUnixNano: fileInfo.ModTime().UnixNano(),
		Sha256:          h.Sum(nil),
	}, nil
}

// MatchesFile reports whether the file at filePath still matches this identity.
//
// It compares size and modification time first, then falls back to a SHA-256
// content comparison so a rewritten-but-identical file, such as a source tree
// re-synced with fresh modification times, does not force a rebuild.
func (id *InputManifest_FileIdentity) MatchesFile(filePath string) (bool, error) {
	current, err := CaptureFileIdentity(filePath)
	if err != nil {
		return false, err
	}
	if id.GetSizeBytes() == current.GetSizeBytes() &&
		id.GetModTimeUnixNano() == current.GetModTimeUnixNano() {
		return true, nil
	}
	return bytes.Equal(id.GetSha256(), current.GetSha256()), nil
}
