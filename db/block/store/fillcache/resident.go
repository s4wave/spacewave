package block_store_fillcache

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"io"
	"iter"
	"os"
	"path/filepath"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// residentMagic opens a resident list file. A file without it reads as empty.
const residentMagic = "spacewave fill cache resident list 1\n"

// residentSlice is the number of listed refs MissingResident checks and
// yields at once, which bounds the memory a read of the list holds.
const residentSlice = 1 << 14

// residentKeyMax bounds one listed ref key. A longer length marks a damaged
// file, and the read stops there.
const residentKeyMax = 1 << 10

// A resident list names the blocks a scope of the decoded-block pool held when
// it was saved, so the next start can read them into the cache before its
// first views ask. It is a hint: a damaged, stale or missing list costs only
// reads, since every listed block is read through the stores that verify it.
// The lists live in a directory beside the database, one file per scope. A
// list outlives a recreated database, since its refs stay good hints.
//
// A file is residentMagic, then each ref's MarshalKey bytes after their
// uvarint length.

// SaveResident replaces scope's resident list with refKeys, each a BlockRef's
// MarshalKey encoding, as DecodedBlockCache.Resident yields them. It writes a
// temporary file and renames it over the list, so a failed save leaves the
// previous list.
func (c *Cache) SaveResident(ctx context.Context, scope string, refKeys iter.Seq[string]) error {
	// Create the list directory and a temporary file in it.
	path := c.residentPath(scope)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.Wrap(err, "create the resident list directory")
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return errors.Wrap(err, "create the resident list")
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// Write the magic and each key after its length.
	w := bufio.NewWriterSize(f, 1<<20)
	_, _ = w.WriteString(residentMagic)
	var length [binary.MaxVarintLen64]byte
	for key := range refKeys {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if key == "" || len(key) > residentKeyMax {
			continue
		}
		_, _ = w.Write(binary.AppendUvarint(length[:0], uint64(len(key))))
		_, _ = w.WriteString(key)
	}

	// Flush the file and rename it over the list.
	if err := w.Flush(); err != nil {
		return errors.Wrap(err, "write the resident list")
	}
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "write the resident list")
	}
	return errors.Wrap(os.Rename(f.Name(), path), "replace the resident list")
}

// MissingResident reads scope's resident list and calls fn with each slice of
// the listed refs the cache does not hold, sorted by key and without
// duplicates, at most residentSlice at a time, and returns the number of refs
// the list named, each counted once per slice it appears in. fn may keep the slice. A missing list calls fn with nothing,
// and a damaged one ends at the damage. A failure to check the cache yields
// the slice whole, as a miss would.
func (c *Cache) MissingResident(ctx context.Context, scope string, fn func([]*block.BlockRef) error) (int, error) {
	// Open the list, reading a missing or foreign file as empty.
	f, err := os.Open(c.residentPath(scope))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, errors.Wrap(err, "open the resident list")
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	magic := make([]byte, len(residentMagic))
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != residentMagic {
		return 0, nil
	}

	// Read the list a slice at a time, passing on the refs the cache lacks.
	var listed int
	for {
		keys, done := readResidentKeys(r, residentSlice)
		refs, n := c.missing(ctx, keys)
		listed += n
		if len(refs) != 0 {
			if err := fn(refs); err != nil {
				return listed, err
			}
		}
		if done || ctx.Err() != nil {
			return listed, context.Cause(ctx)
		}
	}
}

// missing decodes keys into refs, sorted and without duplicates, and returns
// those the cache does not hold, checked in one transaction, with the number
// of refs decoded.
func (c *Cache) missing(ctx context.Context, keys [][]byte) ([]*block.BlockRef, int) {
	// Decode the keys in key order, dropping duplicates and keys that are not
	// refs.
	slices.SortFunc(keys, bytes.Compare)
	keys = slices.CompactFunc(keys, bytes.Equal)
	refs := make([]*block.BlockRef, 0, len(keys))
	for _, key := range keys {
		ref := &block.BlockRef{}
		if ref.UnmarshalVT(key) == nil && !ref.GetEmpty() {
			refs = append(refs, ref)
		}
	}

	// Keep the refs the cache lacks.
	held, err := c.blocks.GetBlockExistsBatch(ctx, refs)
	if err != nil || len(held) != len(refs) {
		return refs, len(refs)
	}
	missing := refs[:0]
	for i, ref := range refs {
		if !held[i] {
			missing = append(missing, ref)
		}
	}
	return missing, len(refs)
}

// readResidentKeys reads up to n keys from r, and reports done at the end of
// the list or at damage.
func readResidentKeys(r *bufio.Reader, n int) ([][]byte, bool) {
	keys := make([][]byte, 0, n)
	for len(keys) < n {
		size, err := binary.ReadUvarint(r)
		if err != nil || size == 0 || size > residentKeyMax {
			return keys, true
		}
		key := make([]byte, size)
		if _, err := io.ReadFull(r, key); err != nil {
			return keys, true
		}
		keys = append(keys, key)
	}
	return keys, false
}

// residentPath returns the list file of scope, in the directory beside the
// database.
func (c *Cache) residentPath(scope string) string {
	return filepath.Join(c.path+".resident", hex.EncodeToString([]byte(scope)))
}
