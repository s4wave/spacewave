//go:build !js

// Package bldr_util_buildcache stores the outputs of deterministic build steps,
// keyed by the step's request and validated by the content of every file the
// step read, so a rebuild that reads the same content replays the outputs
// instead of running the step.
package bldr_util_buildcache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// RootEnv overrides the cache root directory.
	RootEnv = "BLDR_BUILD_CACHE_ROOT"

	// keepEntries is how many input states the cache keeps per key, so both
	// sides of an edit and its revert replay.
	keepEntries = 2

	entryFile  = "entry.bin"
	outputsDir = "out"
	lockFile   = "lock"
	tempPrefix = ".tmp-"
)

// Cache is a directory of build step entries. Each key owns a directory
// holding up to keepEntries entries, one per distinct input state, and a lock
// that serializes steps sharing the key across processes.
type Cache struct {
	root string
}

// Open returns the cache rooted at root.
func Open(root string) *Cache {
	return &Cache{root: root}
}

// Default returns the cache at RootEnv, or under the user cache directory.
func Default() (*Cache, error) {
	if root := os.Getenv(RootEnv); root != "" {
		return Open(root), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, errors.Wrap(err, "resolve build cache root")
	}
	return Open(filepath.Join(dir, "bldr", "build")), nil
}

// Do returns the value of the entry for key whose inputs all still match the
// files on disk, after copying that entry's outputs into outputRoot. Without
// one, it runs build, records the step, and returns its value. Steps with the
// same key run one at a time, so a second waits and replays the first.
func (c *Cache) Do(le *logrus.Entry, key []byte, outputRoot string, build func() (*Step, error)) ([]byte, error) {
	// Serialize the steps that share this key.
	keyDir := c.keyDir(key)
	lock, err := LockFile(filepath.Join(keyDir, lockFile))
	if err != nil {
		return nil, err
	}
	defer lock.Close()

	// Replay the entry whose inputs still match.
	files := newFileHasher()
	if entry, dir := matchEntry(keyDir, files); entry != nil {
		err := restoreOutputs(filepath.Join(dir, outputsDir), outputRoot, entry.GetOutputs())
		if err == nil {
			now := time.Now()
			_ = os.Chtimes(filepath.Join(dir, entryFile), now, now)
			return entry.GetValue(), nil
		}
		le.WithError(err).Warn("rebuilding after a build cache entry failed to restore")
	}

	// Run the step and record it. A failed record costs only the next replay.
	step, err := build()
	if err != nil {
		return nil, err
	}
	if err := storeEntry(keyDir, outputRoot, step, files); err != nil {
		le.WithError(err).Warn("failed to record build cache entry")
	}
	return step.Value, nil
}

// keyDir returns the directory that holds the entries for key.
func (c *Cache) keyDir(key []byte) string {
	name := hex.EncodeToString(key)
	return filepath.Join(c.root, name[:2], name)
}

// matchEntry returns the most recently used entry under keyDir whose inputs
// all match, and its directory.
func matchEntry(keyDir string, files *fileHasher) (*Entry, string) {
	for _, dir := range listEntries(keyDir) {
		data, err := os.ReadFile(filepath.Join(dir, entryFile))
		if err != nil {
			continue
		}
		entry := &Entry{}
		if err := entry.UnmarshalVT(data); err != nil {
			continue
		}
		if slices.ContainsFunc(entry.GetInputs(), func(input *File) bool { return !files.matches(input) }) {
			continue
		}
		return entry, dir
	}
	return nil, ""
}

// listEntries returns the entry directories under keyDir, most recently used
// first.
func listEntries(keyDir string) []string {
	// Collect each complete entry with the time it was last used.
	dirents, _ := os.ReadDir(keyDir)
	type usedEntry struct {
		dir  string
		used time.Time
	}
	var entries []usedEntry
	for _, dirent := range dirents {
		if !dirent.IsDir() || strings.HasPrefix(dirent.Name(), tempPrefix) {
			continue
		}
		dir := filepath.Join(keyDir, dirent.Name())
		info, err := os.Stat(filepath.Join(dir, entryFile))
		if err != nil {
			continue
		}
		entries = append(entries, usedEntry{dir: dir, used: info.ModTime()})
	}

	// Order the entries by last use, newest first.
	slices.SortFunc(entries, func(a, b usedEntry) int { return b.used.Compare(a.used) })
	dirs := make([]string, len(entries))
	for i, entry := range entries {
		dirs[i] = entry.dir
	}
	return dirs
}

// storeEntry records step as an entry under keyDir and drops the entries
// beyond keepEntries.
func storeEntry(keyDir, outputRoot string, step *Step, files *fileHasher) error {
	// Identify every input by its content.
	entry := &Entry{Outputs: step.Outputs, Value: step.Value}
	inputs := slices.Clone(step.Inputs)
	slices.Sort(inputs)
	inputs = slices.Compact(inputs)
	name := sha256.New()
	for _, path := range inputs {
		input, err := files.identify(path)
		if err != nil {
			return errors.Wrapf(err, "identify build input %s", path)
		}
		entry.Inputs = append(entry.Inputs, input)
		_, _ = name.Write([]byte(path))
		_, _ = name.Write(input.GetSha256())
	}
	data, err := entry.MarshalVT()
	if err != nil {
		return err
	}

	// Write the outputs and the entry into a temporary directory.
	tempDir, err := os.MkdirTemp(keyDir, tempPrefix)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	for _, output := range step.Outputs {
		src := filepath.Join(outputRoot, filepath.FromSlash(output))
		if err := copyFile(src, filepath.Join(tempDir, outputsDir, filepath.FromSlash(output))); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(tempDir, entryFile), data, 0o644); err != nil {
		return err
	}

	// Publish the entry by renaming it into place, then drop the least
	// recently used entries.
	dir := filepath.Join(keyDir, hex.EncodeToString(name.Sum(nil)))
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.Rename(tempDir, dir); err != nil {
		return err
	}
	entries := listEntries(keyDir)
	for len(entries) > keepEntries {
		_ = os.RemoveAll(entries[len(entries)-1])
		entries = entries[:len(entries)-1]
	}
	return nil
}

// restoreOutputs copies each recorded output from srcRoot into outputRoot.
func restoreOutputs(srcRoot, outputRoot string, outputs []string) error {
	for _, output := range outputs {
		if !filepath.IsLocal(filepath.FromSlash(output)) {
			return errors.Errorf("build cache output %q escapes the output root", output)
		}
		rel := filepath.FromSlash(output)
		if err := copyFile(filepath.Join(srcRoot, rel), filepath.Join(outputRoot, rel)); err != nil {
			return err
		}
	}
	return nil
}

// copyFile replaces dst with a copy of src, creating its parent directories.
// Removing dst first keeps a hard link to dst from seeing the new content.
func copyFile(src, dst string) error {
	// Open the source and clear the destination path.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Write the copy into a new file.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// fileHasher identifies input files by content, hashing each file at most
// once per Do so that comparing several entries reads each input once.
type fileHasher struct {
	files map[string]*File
}

func newFileHasher() *fileHasher {
	return &fileHasher{files: make(map[string]*File)}
}

// matches reports whether the file at want's path has want's content.
func (h *fileHasher) matches(want *File) bool {
	// Compare sizes first to reject most changed files without a read.
	if cached, ok := h.files[want.GetPath()]; ok {
		return cached.GetSize() == want.GetSize() && bytes.Equal(cached.GetSha256(), want.GetSha256())
	}
	info, err := os.Stat(want.GetPath())
	if err != nil || info.Size() != want.GetSize() {
		return false
	}
	got, err := h.identify(want.GetPath())
	return err == nil && bytes.Equal(got.GetSha256(), want.GetSha256())
}

// identify returns the size and sha256 of the file at path.
func (h *fileHasher) identify(path string) (*File, error) {
	// Return the identity hashed earlier in this Do.
	if cached, ok := h.files[path]; ok {
		return cached, nil
	}

	// Hash the file's content.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sum := sha256.New()
	size, err := io.Copy(sum, f)
	if err != nil {
		return nil, err
	}

	// Remember the identity for the rest of this Do.
	file := &File{Path: path, Size: size, Sha256: sum.Sum(nil)}
	h.files[path] = file
	return file, nil
}
