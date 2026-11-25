package unixfs_checkout

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// FingerprintName is the file inside each published checkout that records
// which roots it holds and the name, size and modification time of each of
// its files when they were written from hash-verified blocks.
const FingerprintName = ".checkout-fingerprint"

// errChanged rejects a checkout whose files are not the ones its
// fingerprint recorded, or that has no fingerprint.
var errChanged = errors.New("its files are not the ones written from the release")

// fingerprint returns a digest of parts and of the name, size and
// modification time of every file below directory but the fingerprint
// itself. A file changes only through a writer, which moves its modification
// time or size, so an equal digest shows the files are the ones it was
// recorded over. It reads only directory entries, never file contents.
func fingerprint(directory string, parts []*block.BlockRef) ([]byte, error) {
	// Bind the digest to the roots the files were written from.
	sum := sha256.New()
	for _, root := range parts {
		_, _ = io.WriteString(sum, root.MarshalString()+"\n")
	}

	// Add each file's name, size and modification time, in lexical order.
	err := filepath.WalkDir(directory, func(name string, entry fs.DirEntry, err error) error {
		// Skip directories and the fingerprint itself.
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(directory, name)
		if err != nil || rel == FingerprintName {
			return err
		}

		// Record the file's entry.
		info, err := entry.Info()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(sum, "%s\x00%d\x00%d\n", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []byte(hex.EncodeToString(sum.Sum(nil))), nil
}

// recordFingerprint writes the fingerprint of staging's files, which the
// caller wrote from hash-verified blocks of parts, into staging.
func recordFingerprint(staging string, parts []*block.BlockRef) error {
	// Record the digest beside the files it covers.
	sum, err := fingerprint(staging, parts)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(staging, FingerprintName), sum, 0o644)
}

// unchanged returns errChanged unless directory's fingerprint matches its
// files and parts. A checkout an older engine wrote has none, so it is
// written again once.
func unchanged(directory string, parts []*block.BlockRef) error {
	// Compare the recorded fingerprint with the files now present.
	recorded, err := os.ReadFile(filepath.Join(directory, FingerprintName))
	if err != nil {
		return errChanged
	}
	current, err := fingerprint(directory, parts)
	if err != nil {
		return err
	}
	if !bytes.Equal(recorded, current) {
		return errChanged
	}
	return nil
}
