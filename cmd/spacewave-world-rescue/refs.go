//go:build !js && !wasip1 && !windows

package main

import (
	"bufio"
	"compress/gzip"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// readRefList reads the block refs of a list in the format of hyperline-admin
// world block-pass --input and the spacewave-pack-copy holes list: one block
// per line as "<store> <hex block key> [<size or reason>]". A line of one
// field takes a hex block key or a base58 block ref. Blank lines and lines
// starting with # are skipped; a gzip file is read through gzip. It calls fn
// with each block key once, in list order.
func readRefList(path string, fn func(key []byte, ref *block.BlockRef) error) error {
	// Open the list, through gzip when it starts with the gzip magic.
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 1<<20)
	var r io.Reader = br
	if magic, _ := br.Peek(2); len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return errors.Wrap(err, path)
		}
		defer zr.Close()
		r = zr
	}

	// Parse each line, skipping repeats.
	seen := make(map[string]struct{})
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ref, err := parseRefLine(line)
		if err != nil {
			return errors.Wrapf(err, "%s:%d", path, n)
		}
		key := packfile.BlockKey(ref.GetHash())
		if _, ok := seen[string(key)]; ok {
			continue
		}
		seen[string(key)] = struct{}{}
		if err := fn(key, ref); err != nil {
			return err
		}
	}
	return sc.Err()
}

// parseRefLine parses one list line into its block ref.
func parseRefLine(line string) (*block.BlockRef, error) {
	// Take the key field: the second of several, or the only one.
	fields := strings.Fields(line)
	field := fields[0]
	if len(fields) >= 2 {
		field = fields[1]
	}

	// Parse it as a hex block key.
	if raw, err := hex.DecodeString(field); err == nil {
		h, err := packfile.ParseBlockKey(raw)
		if err != nil {
			return nil, err
		}
		return block.NewBlockRef(h), nil
	}

	// Else parse it as a b58 block ref.
	ref, err := block.UnmarshalBlockRefB58(field)
	if err != nil {
		return nil, errors.Errorf("not a hex block key or block ref: %q", field)
	}
	if ref == nil || ref.GetHash() == nil {
		return nil, errors.New("empty block ref")
	}
	return ref, nil
}
