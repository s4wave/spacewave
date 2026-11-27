package bldr_manifest_builder

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// CaptureFileIdentity captures the content identity for one file path.
//
// The identity records size, modification time, and SHA-256 digest. Later
// validations always compare the digest. Size and modification time are
// recorded as diagnostics.
func CaptureFileIdentity(filePath string) (*InputManifest_FileIdentity, error) {
	// Stat the file and reject directories.
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if fileInfo.IsDir() {
		return nil, errors.Errorf("path is a directory: %s", filePath)
	}

	// Open the file for hashing.
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Hash the contents and validate the recorded size.
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return nil, err
	}
	if fileInfo.Size() < 0 {
		return nil, errors.Errorf("negative file size: %d", fileInfo.Size())
	}

	// Return the identity with the digest and diagnostics.
	return &InputManifest_FileIdentity{
		// #nosec G115 -- fileInfo.Size() is validated as non-negative immediately above.
		SizeBytes:       uint64(fileInfo.Size()),
		ModTimeUnixNano: fileInfo.ModTime().UnixNano(),
		Sha256:          h.Sum(nil),
	}, nil
}

// CaptureIdentity captures the identity of the input at filePath: the file's
// content, or for a DirSuffix input the names of the matching files.
func (f *InputManifest_File) CaptureIdentity(filePath string) (*InputManifest_FileIdentity, error) {
	if suffix := f.GetDirSuffix(); suffix != "" {
		return captureDirIdentity(filePath, suffix)
	}
	return CaptureFileIdentity(filePath)
}

// MatchesPath reports whether the input at filePath still matches the
// identity f recorded.
func (f *InputManifest_File) MatchesPath(filePath string) (bool, error) {
	// An identity without a digest cannot match.
	recorded := f.GetIdentity()
	if len(recorded.GetSha256()) == 0 {
		return false, nil
	}

	// Capture the current identity and compare digests.
	current, err := f.CaptureIdentity(filePath)
	if err != nil {
		return false, err
	}
	return bytes.Equal(recorded.GetSha256(), current.GetSha256()), nil
}

// captureDirIdentity captures the identity of the files directly in dirPath
// whose names end in suffix: the digest of their sorted names, with their
// count as the size. Their contents are not read.
func captureDirIdentity(dirPath, suffix string) (*InputManifest_FileIdentity, error) {
	// List the matching file names in order.
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, ent := range entries {
		if !ent.IsDir() && strings.HasSuffix(ent.Name(), suffix) {
			names = append(names, ent.Name())
		}
	}
	slices.Sort(names)

	// Hash the names, each ended by a newline.
	h := sha256.New()
	for _, name := range names {
		_, _ = io.WriteString(h, name+"\n")
	}
	return &InputManifest_FileIdentity{
		SizeBytes: uint64(len(names)),
		Sha256:    h.Sum(nil),
	}, nil
}

// MatchesFile reports whether the file at filePath still matches this identity.
//
// It compares the SHA-256 digest on every validation. Size and modification
// time remain recorded as diagnostics but are not trusted as content identity.
func (id *InputManifest_FileIdentity) MatchesFile(filePath string) (bool, error) {
	// An identity without a digest cannot match.
	if len(id.GetSha256()) == 0 {
		return false, nil
	}

	// Capture the current identity and compare digests.
	current, err := CaptureFileIdentity(filePath)
	if err != nil {
		return false, err
	}
	return bytes.Equal(id.GetSha256(), current.GetSha256()), nil
}
