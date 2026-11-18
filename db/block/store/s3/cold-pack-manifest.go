//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"container/heap"
	"context"
	"crypto/sha256"
	"io"
	"io/fs"
	"iter"
	"math"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
)

// BuildColdPackManifest checks a local mirror of the exported bucket objects.
// It needs no bucket access. The mirror is rooted at the configured cold
// prefix and uses the writer's layout: XNN/entries/<id> and XNN/packs/<id>,
// with no extension. exported yields the canonical block keys in strict order.
// The builder matches the exported set exactly, including counts. It holds raw
// tails only for intervals active in the merge, not one row per exported key.
func BuildColdPackManifest(ctx context.Context, dir string, exported iter.Seq2[[]byte, error]) (*packfile.ColdPackManifest, error) {
	// Open the local mirror without following paths outside it.
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close() // Read-only directory handle.

	// Discover the object names and independently derive their tail bounds.
	manifest := &packfile.ColdPackManifest{}
	err = fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		// Pack payload files are opened by the name in the matching entry.
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() && d.Name() == strings.TrimSuffix(packDir, "/") {
			return fs.SkipDir
		}
		if d.IsDir() || path.Base(path.Dir(name)) != strings.TrimSuffix(entryDir, "/") {
			return nil
		}

		// Read one entry and the tail of its pack, then release their bytes.
		meta, err := inspectColdEntry(root, name)
		if err != nil {
			return errors.Wrap(err, name)
		}
		manifest.Entries = append(manifest.Entries, meta)
		if len(manifest.Entries) > 1<<16 {
			return errors.New("cold manifest exceeds 65536 packs")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Assign stable pack numbers and verify the key stream before publication.
	slices.SortFunc(manifest.Entries, compareColdPacks)
	catalog, err := newColdPackCatalog("", manifest)
	if err != nil {
		return nil, err
	}
	if err := verifyColdExport(ctx, root, catalog, exported); err != nil {
		return nil, err
	}
	return manifest, nil
}

// inspectColdEntry pairs the writer's metadata object with its local pack.
func inspectColdEntry(root *os.Root, name string) (*packfile.ColdPackEntry, error) {
	// Derive the subpath from the on-disk object key, not from a caller's guess.
	id := path.Base(name)
	if err := identity.ValidatePackID(id); err != nil {
		return nil, err
	}
	subpath := path.Dir(path.Dir(name))
	if subpath == "." {
		subpath = ""
	} else {
		subpath += "/"
	}

	// Decode the writer’s entry and check it describes this object name.
	raw, err := root.ReadFile(name)
	if err != nil {
		return nil, err
	}
	entry := &packfile.PackfileEntry{}
	if err := entry.UnmarshalVT(raw); err != nil {
		return nil, err
	}
	if entry.Id != id || entry.BlockCount == 0 {
		return nil, errors.New("entry pack ID or count disagrees with its object")
	}

	// Bind the manifest to the pack's exact tail and inclusive endpoints.
	meta := &packfile.ColdPackEntry{Subpath: subpath, Entry: entry}
	tail, err := readLocalColdTail(root, meta)
	if err != nil {
		return nil, err
	}
	reader, err := kvfile.BuildReaderWithIndexTail(tail, entry.SizeBytes)
	if err != nil {
		return nil, err
	}
	if reader.Size() != entry.BlockCount {
		return nil, errors.Errorf("pack has %d index keys; metadata reports %d", reader.Size(), entry.BlockCount)
	}

	// Read the inclusive key interval from kvfile order.
	first, err := reader.ReadIndexEntry(0)
	if err != nil {
		return nil, err
	}
	last, err := reader.ReadIndexEntry(entry.BlockCount - 1)
	if err != nil {
		return nil, err
	}

	// Bind the exact tail bytes to the entry the mounted store will trust.
	digest := sha256.Sum256(tail)
	meta.FirstKey, meta.LastKey = first.Key, last.Key
	meta.IndexSha256 = digest[:]
	meta.IndexTailBytes = uint64(len(tail))
	return meta, nil
}

// readLocalColdTail reads just the raw index of a verified local file size.
func readLocalColdTail(root *os.Root, meta *packfile.ColdPackEntry) ([]byte, error) {
	// Open the immutable mirror's pack, not a path from within its metadata.
	key := meta.Subpath + packDir + meta.Entry.Id
	f, err := root.Open(key)
	if err != nil {
		return nil, err
	}
	defer f.Close() // Read-only pack handle.
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := meta.Entry.SizeBytes
	if !info.Mode().IsRegular() || size > math.MaxInt64 || info.Size() != int64(size) {
		return nil, errors.Errorf("pack %q does not match its declared file size", key)
	}

	// Refuse a corrupt trailer that would include the payload or allocate
	// more than the number of entries and kvfile's maximum entry size allow.
	base, err := kvfile.IndexTailOffset(f, size)
	if err != nil {
		return nil, err
	}
	count := meta.Entry.BlockCount
	if base == 0 || base >= size || count > math.MaxUint32 || size-base > count*(2048+10+8)+8 {
		return nil, errors.New("local cold pack tail exceeds its entry bounds")
	}
	return io.ReadAll(io.NewSectionReader(f, int64(base), int64(size-base))) //nolint:gosec // base and length fit the checked file size.
}

// verifyColdExport merge-joins the sorted export with all pack indexes. An
// interval and a bloom positive alone are not proof that a key was exported.
func verifyColdExport(ctx context.Context, root *os.Root, catalog *coldPackCatalog, exported iter.Seq2[[]byte, error]) error {
	// Keep only the current record of each overlapping pack and its raw tail.
	var active coldBuildHeap
	var next int
	var seen, expected uint64
	var previous []byte
	for i := range catalog.packs {
		expected += catalog.packs[i].meta.Entry.BlockCount
	}

	// Match each key against the globally earliest index entry.
	for key, err := range exported {
		// Validate the stream independently of the pack-derived bounds.
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !coldPackKey(key) || bytes.Compare(previous, key) >= 0 {
			return errors.Errorf("exported key %d is invalid, duplicate or out of order", seen+1)
		}

		// Open each pack on entering its inclusive interval. No pack can go
		// unnoticed in a gap: it joins the heap with its first unconsumed key.
		for next < len(catalog.intervals) {
			pack := &catalog.packs[catalog.intervals[next].pack]
			if bytes.Compare(pack.meta.FirstKey, key) > 0 {
				break
			}
			cursor, err := openColdBuildCursor(root, pack)
			if err != nil {
				return errors.Wrap(err, pack.key)
			}
			heap.Push(&active, cursor)
			next++
		}

		// Validate exact membership and the original writer's bloom for this key.
		if len(active) == 0 || !bytes.Equal(active[0].entry.Key, key) {
			return errors.Errorf("exported key %x (line %d) does not match the pack set", key, seen+1)
		}
		if err := active.consume(); err != nil {
			return err
		}
		previous = append(previous[:0], key...)
		seen++
	}

	// No pack may contain a key missing from the exported set, nor vice versa.
	if seen != expected || len(active) != 0 || next != len(catalog.packs) {
		return errors.Errorf("exported %d keys; packs declare %d", seen, expected)
	}
	return nil
}
