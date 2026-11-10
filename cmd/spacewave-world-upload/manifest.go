package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// statusOK is the one manifest status counted as done.
const statusOK = "200"

// done is a key the manifest records as put with status 200.
type done struct {
	size   int64
	sha256 string
}

// readManifest returns the keys the manifest at path records with status
// 200, without locking it. A missing file records none. A torn last line is
// dropped with a warning to warn.
func readManifest(path string, warn io.Writer) (map[string]done, error) {
	// Read the file; a missing one records nothing.
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]done{}, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "--manifest")
	}
	out, _, err := parseManifest(path, raw, warn)
	return out, err
}

// parseManifest returns the keys raw records with status 200 and the length
// of its whole lines. A last line without its newline is torn by a crash
// mid-write: it is dropped with a warning to warn. It refuses any whole line
// it cannot parse.
func parseManifest(path string, raw []byte, warn io.Writer) (map[string]done, int, error) {
	// Drop a torn last line.
	whole := bytes.LastIndexByte(raw, '\n') + 1
	if whole != len(raw) {
		_, _ = fmt.Fprintf(warn, "warning: --manifest %s: dropping its torn last line of %d bytes, %q\n", path, len(raw)-whole, raw[whole:])
	}
	out := make(map[string]done)
	if whole == 0 {
		return out, 0, nil
	}

	// Parse each line, keeping the keys put with status 200.
	for i, line := range strings.Split(string(raw[:whole-1]), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 6 {
			return nil, 0, errors.Errorf("--manifest %s line %d has %d fields, want 6", path, i+1, len(f))
		}
		size, err := strconv.ParseInt(f[2], 10, 64)
		if err != nil {
			return nil, 0, errors.Errorf("--manifest %s line %d: bad size", path, i+1)
		}
		if f[5] == statusOK {
			out[f[1]] = done{size: size, sha256: f[3]}
		}
	}
	return out, whole, nil
}

// manifest is the run's manifest file, held under an exclusive lock for the
// whole run. It appends one fsynced line per object put.
type manifest struct {
	path     string
	f        *os.File
	writable bool
	now      func() time.Time
}

// openManifest opens and locks path. Without dryRun it opens to append,
// creating the file, and without resume the file must be empty, so a second
// run does not PUT again what the first did. With dryRun it opens an
// existing file only to read, and returns nil for a missing one. It refuses
// a manifest another process holds locked.
func openManifest(path string, resume, dryRun bool) (*manifest, error) {
	// Open the file: read-only for a dry run, else to append.
	flag, writable := os.O_RDWR|os.O_CREATE|os.O_APPEND, true
	if dryRun {
		flag, writable = os.O_RDONLY, false
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if dryRun && errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "--manifest")
	}

	// Take the lock, then refuse a used manifest without resume.
	err = lockFile(f)
	if err != nil {
		err = errors.Wrapf(err, "--manifest %s", path)
	}
	if err == nil && writable && !resume {
		var st os.FileInfo
		if st, err = f.Stat(); err == nil && st.Size() != 0 {
			err = errors.Errorf("--manifest %s is not empty; pass --resume to continue it", path)
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &manifest{path: path, f: f, writable: writable, now: time.Now}, nil
}

// load returns the keys the manifest records with status 200. A torn last
// line is dropped with a warning and, when the manifest is writable, cut off
// the file, so the next line appended starts a line of its own.
func (m *manifest) load(warn io.Writer) (map[string]done, error) {
	// Read the whole file through the locked handle.
	st, err := m.f.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "--manifest")
	}
	raw := make([]byte, st.Size())
	if _, err := m.f.ReadAt(raw, 0); err != nil && err != io.EOF {
		return nil, errors.Wrap(err, "--manifest")
	}
	out, whole, err := parseManifest(m.path, raw, warn)
	if err != nil {
		return nil, err
	}

	// Cut a torn last line off a writable manifest.
	if m.writable && whole != len(raw) {
		if err := m.f.Truncate(int64(whole)); err != nil {
			return nil, errors.Wrap(err, "cut torn manifest line")
		}
		if err := m.f.Sync(); err != nil {
			return nil, errors.Wrap(err, "sync manifest")
		}
	}
	return out, nil
}

// record appends and syncs the line of one object put with its status.
func (m *manifest) record(obj *object, status string) error {
	// Format the line.
	line := strings.Join([]string{
		m.now().UTC().Format(time.RFC3339Nano),
		obj.key,
		strconv.FormatInt(obj.size, 10),
		hex.EncodeToString(obj.sha256[:]),
		hex.EncodeToString(obj.md5),
		status,
	}, "\t") + "\n"

	// Write and sync it.
	if _, err := m.f.WriteString(line); err != nil {
		return errors.Wrap(err, "write manifest")
	}
	return errors.Wrap(m.f.Sync(), "sync manifest")
}

// close releases the lock and closes the manifest.
func (m *manifest) close() error {
	return m.f.Close()
}
