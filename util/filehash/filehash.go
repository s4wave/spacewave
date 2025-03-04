//go:build !js

package bldr_util_filehash

import (
	"encoding/base32"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/zeebo/blake3"
	"slices"
)

// HashFileWithBlake3 hashes a file using blake3 and returns the first 8 characters of the base32-encoded hash.
// This is similar to the approach used in dist/compiler/bundle.go.
func HashFileWithBlake3(filePath string) (string, error) {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return "", errors.Wrap(err, "failed to read file for hashing")
	}

	// Create a hash
	hasher := blake3.New()
	_, err = hasher.Write(fileData)
	if err != nil {
		return "", errors.Wrap(err, "failed to hash file")
	}

	// Get the hash and encode it
	hash := hasher.Sum(nil)
	hashStr := base32.StdEncoding.EncodeToString(hash)[:8]
	return hashStr, nil
}

// AddHashToFilename adds a hash to a filename before the extension.
// For example: "example.js" becomes "example-abc123.js"
func AddHashToFilename(filename, hash string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	return base + "-" + hash + ext
}

// UpdateSourceMapReference updates the sourceMappingURL in a file.
// It removes any existing sourceMappingURL and adds a new one at the end.
func UpdateSourceMapReference(filePath, newMapFilename string) error {
	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to read file for updating source map")
	}

	// Remove any existing sourceMappingURL
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//# sourceMappingURL=") {
			lines = slices.Delete(lines, i, i+1)
			break
		}
	}

	// Join the lines without the sourcemap
	content = []byte(strings.Join(lines, "\n"))

	// Add the new sourceMappingURL at the end
	// If content doesn't end with newline, add one
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		content = append(content, '\n')
	}
	content = append(content, []byte("//# sourceMappingURL="+filepath.Base(newMapFilename))...)

	// Write the updated content back to the file
	err = os.WriteFile(filePath, content, 0o644)
	if err != nil {
		return errors.Wrap(err, "failed to write updated file with source map reference")
	}

	return nil
}
