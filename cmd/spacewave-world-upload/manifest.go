package main

import (
	"bytes"
	"encoding/hex"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// done is a key the manifest records as put with status 200.
type done struct {
	size   int64
	sha256 string
}

// readManifest returns the keys the manifest at path records with status
// 200. A missing file records none. It refuses a line it cannot parse, a
// last line without its newline included.
func readManifest(path string) (map[string]done, error) {
	// Read the file; a missing or empty one records nothing.
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]done{}, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "--manifest")
	}
	out := make(map[string]done)
	if len(raw) == 0 {
		return out, nil
	}

	// Parse each line, keeping the keys put with status 200.
	if raw[len(raw)-1] != '\n' {
		return nil, errors.Errorf("--manifest %s does not end in a newline; its last line is torn", path)
	}
	for i, line := range strings.Split(string(bytes.TrimSuffix(raw, []byte("\n"))), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 6 {
			return nil, errors.Errorf("--manifest %s line %d has %d fields, want 6", path, i+1, len(f))
		}
		size, err := strconv.ParseInt(f[2], 10, 64)
		if err != nil {
			return nil, errors.Errorf("--manifest %s line %d: bad size", path, i+1)
		}
		if f[5] == "200" {
			out[f[1]] = done{size: size, sha256: f[3]}
		}
	}
	return out, nil
}

// manifest appends one fsynced line per object put.
type manifest struct {
	f   *os.File
	now func() time.Time
}

// openManifest opens path to append. Without resume the file must be absent
// or empty, so a second run does not PUT again what the first did.
func openManifest(path string, resume bool) (*manifest, error) {
	// Open to append, refusing a used manifest without resume.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, errors.Wrap(err, "--manifest")
	}
	st, err := f.Stat()
	if err == nil && st.Size() != 0 && !resume {
		err = errors.Errorf("--manifest %s is not empty; pass --resume to continue it", path)
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &manifest{f: f, now: time.Now}, nil
}

// record appends and syncs the line of one object put.
func (m *manifest) record(obj *object, status int) error {
	line := strings.Join([]string{
		m.now().UTC().Format(time.RFC3339Nano),
		obj.key,
		strconv.FormatInt(obj.size, 10),
		hex.EncodeToString(obj.sha256[:]),
		hex.EncodeToString(obj.md5),
		strconv.Itoa(status),
	}, "\t") + "\n"
	if _, err := m.f.WriteString(line); err != nil {
		return errors.Wrap(err, "write manifest")
	}
	return errors.Wrap(m.f.Sync(), "sync manifest")
}

// close closes the manifest.
func (m *manifest) close() error {
	return m.f.Close()
}
