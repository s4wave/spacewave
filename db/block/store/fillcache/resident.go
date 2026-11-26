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
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// residentMagic opens a resident list file. A file without it reads as empty.
const residentMagic = "spacewave fill cache resident list 1\n"

// residentDir is the directory of the lists, inside the cache's directory.
const residentDir = "resident"

// residentSlice bounds the number of refs processed at once from a list.
const residentSlice = 1 << 14

// residentKeyMax bounds one listed ref key. A longer length marks a damaged
// file, and the read stops there.
const residentKeyMax = 1 << 10

// A resident list names the blocks a scope of the decoded-block pool held when
// it was saved, so the next start can read them into the cache before its
// first views ask. It is a hint: a damaged, stale or missing list costs only
// reads, since every listed block is read through the stores that verify it.
// The lists live in the cache's directory beside the packs, one file per
// scope.
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
// the slice whole, as a miss would. Call it once the packs have loaded:
// before then every ref reads as missing.
func (c *Cache) MissingResident(ctx context.Context, scope string, fn func([]*block.BlockRef) error) (int, error) {
	// Open the list, reading a missing or foreign file as empty.
	f, r, err := openResident(c.residentPath(scope))
	if err != nil || f == nil {
		return 0, err
	}
	defer f.Close()

	// Read the list a slice at a time, passing on the refs the cache lacks.
	var listed int
	for {
		keys, done := readResidentKeys(r, residentSlice)
		refs, n := c.missing(keys)
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

// markResident restores reads from every saved resident list before eviction.
func (c *Cache) markResident(ctx context.Context, mark func(keys [][]byte)) {
	// Open the directory only if the cache still needs the lists.
	if ctx.Err() != nil {
		return
	}
	dir := filepath.Join(c.dir, residentDir)
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer d.Close()

	// Walk the directory in batches, skipping unfinished saves.
	for ctx.Err() == nil {
		entries, err := d.ReadDir(64)
		for _, entry := range entries {
			if ctx.Err() != nil {
				return
			}
			if !entry.IsDir() && !strings.HasSuffix(entry.Name(), ".tmp") {
				markResidentFile(ctx, filepath.Join(dir, entry.Name()), mark)
			}
		}
		if err != nil {
			return
		}
	}
}

// markResidentFile marks the pack keys of the refs read from one list.
func markResidentFile(ctx context.Context, path string, mark func(keys [][]byte)) {
	// Ignore lists that cannot supply any hints.
	f, r, err := openResident(path)
	if err != nil || f == nil {
		return
	}
	defer f.Close()

	// Decode one slice at a time so each mark holds the Log's lock briefly.
	for ctx.Err() == nil {
		keys, done := readResidentKeys(r, residentSlice)
		packKeys := make([][]byte, 0, len(keys))
		for _, key := range keys {
			var ref block.BlockRef
			if ref.UnmarshalVT(key) == nil && !ref.GetEmpty() {
				packKeys = append(packKeys, packfile.BlockKey(ref.GetHash()))
			}
		}
		if len(packKeys) != 0 && ctx.Err() == nil {
			mark(packKeys)
		}
		if done {
			return
		}
	}
}

// openResident opens a resident list and positions the reader after the magic.
// A missing or foreign file returns nil for the file, reader and error.
func openResident(path string) (*os.File, *bufio.Reader, error) {
	// Open the saved file if it still exists.
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, errors.Wrap(err, "open the resident list")
	}

	// Hand off the file only when it starts with the list magic.
	r := bufio.NewReaderSize(f, 1<<20)
	var magic [len(residentMagic)]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil || string(magic[:]) != residentMagic {
		_ = f.Close()
		return nil, nil, nil
	}
	return f, r, nil
}

// missing decodes keys into refs, sorted and without duplicates, and returns
// those the cache does not hold, with the number of refs decoded.
func (c *Cache) missing(keys [][]byte) ([]*block.BlockRef, int) {
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
	missing := refs[:0]
	for _, ref := range refs {
		if !c.packs.Has(packfile.BlockKey(ref.GetHash())) {
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

// removeResidentTemps deletes the temporary files a killed SaveResident left
// in the list directory of the cache in dir. A save that is running when it
// is called loses its file, fails its rename and keeps the previous list.
// Leftovers it cannot delete cost only disk, so it ignores failures.
func removeResidentTemps(dir string) {
	temps, _ := filepath.Glob(filepath.Join(dir, residentDir, "*.tmp"))
	for _, name := range temps {
		_ = os.Remove(name)
	}
}

// residentPath returns the list file of scope.
func (c *Cache) residentPath(scope string) string {
	return filepath.Join(c.dir, residentDir, hex.EncodeToString([]byte(scope)))
}
