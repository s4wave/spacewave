//go:build !tinygo

package block_store_s3

import (
	"bufio"
	"bytes"
	"cmp"
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
// The builder matches the exported set exactly, allowing copies in distinct packs. It holds raw
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

		// Verify one pack before retaining only its manifest entry.
		meta, err := inspectColdEntry(ctx, root, name)
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
func inspectColdEntry(ctx context.Context, root *os.Root, name string) (*packfile.ColdPackEntry, error) {
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

	// Open the pack paired with this entry; one handle supplies tail and payloads.
	meta := &packfile.ColdPackEntry{Subpath: subpath, Entry: entry}
	f, err := root.Open(subpath + packDir + id)
	if err != nil {
		return nil, err
	}
	defer f.Close() // Read-only pack handle.

	// Bind the manifest to the pack's exact tail and count.
	tail, err := readLocalColdTail(f, meta)
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

	// Publish a whole-pack digest only after verifying every indexed payload.
	meta.PackSha256, err = verifyColdPayloads(ctx, f, reader, tail, entry.SizeBytes-meta.IndexTailBytes)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

// readLocalColdTail reads just the raw index of a verified local file size.
func readLocalColdTail(f *os.File, meta *packfile.ColdPackEntry) ([]byte, error) {
	// Read only regular files that match the writer's recorded pack length.
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := meta.Entry.SizeBytes
	if !info.Mode().IsRegular() || size > math.MaxInt64 || info.Size() != int64(size) {
		return nil, errors.Errorf("pack %q does not match its declared file size", f.Name())
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

// verifyColdPayloads reads each extent once in physical order, hashing the
// exact bytes it checks. The kvfile writer lays extents end-to-end, with the
// sorted index after them. Only one pack's offsets and one value are live.
func verifyColdPayloads(ctx context.Context, f *os.File, r *kvfile.Reader, tail []byte, base uint64) ([]byte, error) {
	// Gather this pack's physical order; the index is sorted by key instead.
	extents := make([]*kvfile.IndexEntry, 0, r.Size())
	for pos := range r.Size() {
		e, err := r.ReadIndexEntry(pos)
		if err != nil {
			return nil, err
		}
		extents = append(extents, e)
	}
	slices.SortFunc(extents, func(a, b *kvfile.IndexEntry) int { return cmp.Compare(a.Offset, b.Offset) })

	// Hash the same sequential payload stream the block decoder consumes.
	digest := sha256.New()
	section := io.NewSectionReader(f, 0, int64(base)) //nolint:gosec // The tail bounds fit the checked file size.
	payload := io.TeeReader(bufio.NewReaderSize(section, 128<<10), digest)
	var buf []byte
	var off uint64
	for _, e := range extents {
		// Refuse overlapping, gapped or out-of-bounds writer extents.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if e.Offset != off || e.Size == 0 || e.Size > math.MaxInt || e.Size > base-off {
			return nil, errors.Errorf("invalid cold pack extent at %d (size %d, expected offset %d)", e.Offset, e.Size, off)
		}

		// Decode the stored block and VerifyRead against its full index hash.
		n := int(e.Size) //nolint:gosec // The extent fits in an int.
		buf = slices.Grow(buf[:0], n)[:n]
		if _, err := io.ReadFull(payload, buf); err != nil {
			return nil, err
		}
		if _, _, err := packfile.DecodeBlockValue(e.Key, buf); err != nil {
			return nil, errors.Wrapf(err, "verify cold extent at %d", off)
		}
		off += e.Size
	}

	// The digest includes the exact index bytes used for the verification.
	if off != base {
		return nil, errors.New("cold extents do not cover the pack payload")
	}
	_, _ = digest.Write(tail)
	return digest.Sum(nil), nil
}

// verifyColdExport merge-joins the sorted export with all pack indexes. An
// interval and a bloom positive alone are not proof that a key was exported.
func verifyColdExport(ctx context.Context, root *os.Root, catalog *coldPackCatalog, exported iter.Seq2[[]byte, error]) error {
	// Keep only the current record of each overlapping pack and its raw tail.
	var active coldBuildHeap
	var next int
	var seen, duplicates, expected uint64
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
		for copies := 0; len(active) != 0 && bytes.Equal(active[0].entry.Key, key); copies++ {
			// Verify every pack's copy; the exported key names the set, not its multiplicity.
			if err := active.consume(); err != nil {
				return err
			}
			if copies != 0 {
				duplicates++
			}
		}
		previous = append(previous[:0], key...)
		seen++
	}

	// No pack may contain a key missing from the exported set, nor vice versa.
	if seen+duplicates != expected || len(active) != 0 || next != len(catalog.packs) {
		return errors.Errorf("exported %d keys with %d duplicate pack entries; packs declare %d", seen, duplicates, expected)
	}
	return nil
}
