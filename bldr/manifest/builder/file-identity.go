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
// The identity records size, modification time, and SHA-256 digest. Later
// validations always compare the digest; size and modification time are
// recorded as diagnostics.
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
// It compares the SHA-256 digest on every validation. Size and modification
// time remain recorded as diagnostics but are not trusted as content identity.
func (id *InputManifest_FileIdentity) MatchesFile(filePath string) (bool, error) {
	if len(id.GetSha256()) == 0 {
		return false, nil
	}
	current, err := CaptureFileIdentity(filePath)
	if err != nil {
		return false, err
	}
	return bytes.Equal(id.GetSha256(), current.GetSha256()), nil
}
