package packcopy

import (
	"bufio"
	"context"
	"encoding/hex"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// UnknownRefs is a source that withholds the refs of listed blocks. A pack
// cannot say a block's refs are unknown, so one written with empty refs reads
// as a leaf; the list marks those blocks and the copy then takes them only
// from a source that knows their refs.
type UnknownRefs struct {
	Source
	// listed holds the listed blocks by pack index key.
	listed map[string]struct{}
}

// NewUnknownRefs wraps src with a list of hex hashes, one block per line. A
// line is the hash alone, or the hash after a store name; fields after the
// hash are ignored.
func NewUnknownRefs(src Source, list io.Reader) (*UnknownRefs, error) {
	u := &UnknownRefs{Source: src, listed: make(map[string]struct{})}
	lines := bufio.NewScanner(list)
	for lines.Scan() {
		// Find the hash.
		fields := strings.Fields(lines.Text())
		if len(fields) == 0 {
			continue
		}
		field := fields[0]
		if len(fields) > 1 {
			field = fields[1]
		}

		// Store it.
		key, err := hex.DecodeString(field)
		if err != nil {
			return nil, errors.Wrap(err, "parse unknown refs list")
		}
		u.listed[string(key)] = struct{}{}
	}
	return u, lines.Err()
}

// GetStoredBlock reads the block, marking a listed block's refs unknown.
func (u *UnknownRefs) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := u.Source.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return stored, err
	}
	if _, ok := u.listed[string(packfile.BlockKey(ref.GetHash()))]; ok {
		return &block.StoredBlock{Data: stored.Data}, nil
	}
	return stored, nil
}

// _ is a type assertion.
var _ Source = (*UnknownRefs)(nil)
