package main

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"syscall"

	"github.com/pkg/errors"
)

// maxCredsBytes is the largest creds file read.
const maxCredsBytes = 64 << 10

// creds are the access key id and secret. No error or log names their values.
type creds struct {
	accessKeyID, secretAccessKey string
}

// readCreds reads the creds file at path. It refuses anything but a regular
// file of mode 0600 owned by the user. Errors name the path and line numbers
// alone, never the contents.
func readCreds(path string) (*creds, error) {
	// Open without following a symlink and check the open file itself.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errors.Wrap(err, "--creds")
	}
	defer f.Close()

	// Require a regular file of mode 0600.
	st, err := f.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "--creds")
	}
	if !st.Mode().IsRegular() {
		return nil, errors.Errorf("--creds %s is not a regular file", path)
	}
	if st.Mode().Perm() != 0o600 {
		return nil, errors.Errorf("--creds %s has mode %04o, want 0600", path, st.Mode().Perm())
	}

	// Require the user to own it.
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errors.Errorf("--creds %s: cannot read its owner", path)
	}
	if int(sys.Uid) != os.Getuid() {
		return nil, errors.Errorf("--creds %s is owned by uid %d, not the user's %d", path, sys.Uid, os.Getuid())
	}

	// Read the key=value lines.
	raw, err := io.ReadAll(io.LimitReader(f, maxCredsBytes+1))
	if err != nil {
		return nil, errors.Wrap(err, "--creds")
	}
	if len(raw) > maxCredsBytes {
		return nil, errors.Errorf("--creds %s is over %d bytes", path, maxCredsBytes)
	}
	c := &creds{}
	for i, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		k, v, ok := bytes.Cut(line, []byte("="))
		k, v = bytes.TrimSpace(k), bytes.TrimSpace(v)
		var dst *string
		switch {
		case !ok:
			return nil, errors.Errorf("--creds %s line %d is not key=value", path, i+1)
		case string(k) == "access_key_id":
			dst = &c.accessKeyID
		case string(k) == "secret_access_key":
			dst = &c.secretAccessKey
		default:
			return nil, errors.Errorf("--creds %s line %d has an unknown key", path, i+1)
		}
		if *dst != "" {
			return nil, errors.Errorf("--creds %s line %d repeats a key", path, i+1)
		}
		if len(v) == 0 {
			return nil, errors.Errorf("--creds %s line %d has an empty value", path, i+1)
		}
		*dst = string(v)
	}
	if c.accessKeyID == "" || c.secretAccessKey == "" {
		return nil, errors.Errorf("--creds %s needs both access_key_id and secret_access_key", path)
	}
	return c, nil
}

// String hides the values from any print of the creds.
func (c *creds) String() string {
	return "creds(" + strconv.Itoa(len(c.accessKeyID)) + "-byte id, secret hidden)"
}
