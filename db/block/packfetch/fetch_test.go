package packfetch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/packfile"
)

// fakeSource serves the blocks it holds, as the SFTP pack source would, and
// fails every read after failAfter reads when failAfter is positive.
type fakeSource struct {
	mu        sync.Mutex
	blocks    map[string]*block.StoredBlock
	reads     int
	failAfter int
}

// errKilled stands in for a kill of the fetcher.
var errKilled = errors.New("killed")

// GetStoredBlock returns the block, or nil when the source lacks it.
func (s *fakeSource) GetStoredBlock(_ context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	if s.failAfter > 0 && s.reads > s.failAfter {
		return nil, errKilled
	}
	stored := s.blocks[ref.MarshalString()]
	if stored == nil {
		return nil, nil
	}
	c := *stored
	return &c, nil
}

// fakeBucket is an in-memory bucket that records every request, and fails
// every put after failPutsAfter puts when it is positive.
type fakeBucket struct {
	mu            sync.Mutex
	objects       map[string][]byte
	puts          int
	failPutsAfter int
	deletes       int
	overwrites    int
}

func newFakeBucket() *fakeBucket { return &fakeBucket{objects: make(map[string][]byte)} }

func (b *fakeBucket) PutObject(_ context.Context, _, key string, data []byte, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failPutsAfter > 0 && b.puts >= b.failPutsAfter {
		return errKilled
	}
	b.puts++
	if _, ok := b.objects[key]; ok {
		b.overwrites++
	}
	b.objects[key] = bytes.Clone(data)
	return nil
}

func (b *fakeBucket) GetObject(_ context.Context, _, key string) (io.ReadCloser, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data, ok := b.objects[key]
	if !ok {
		return nil, block_store_s3.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(bytes.Clone(data))), nil
}

func (b *fakeBucket) HeadObject(_ context.Context, _, key string) (*block_store_s3.ObjectStat, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data, ok := b.objects[key]
	if !ok {
		return nil, block_store_s3.ErrNotFound
	}
	return &block_store_s3.ObjectStat{Size: int64(len(data))}, nil
}

// DeleteObject is never called: the Bucket interface has no delete, and a
// fetcher that found a way to reach this would be counted.
func (b *fakeBucket) DeleteObject(context.Context, string, string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deletes++
	return nil
}

// keysUnder lists the object keys under prefix, sorted.
func (b *fakeBucket) keysUnder(prefix string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var keys []string
	for k := range b.objects {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, strings.TrimPrefix(k, prefix))
		}
	}
	slices.Sort(keys)
	return keys
}

// newBlocks builds n blocks with refs between some of them, and a source
// holding them.
func newBlocks(t *testing.T, n int) (*fakeSource, []*block.BlockRef) {
	t.Helper()
	src := &fakeSource{blocks: make(map[string]*block.StoredBlock)}
	var refs []*block.BlockRef
	for i := range n {
		data := []byte(fmt.Sprintf("live block %d of the world", i))
		var children []*block.BlockRef
		if i > 0 {
			children = []*block.BlockRef{refs[i-1]}
		}
		ref, err := block.BuildBlockRef(data, &block.PutOpts{})
		if err != nil {
			t.Fatal(err)
		}
		src.blocks[ref.MarshalString()] = &block.StoredBlock{Data: data, Refs: children, RefsKnown: true}
		refs = append(refs, ref)
	}
	return src, refs
}

const testPrefix = "world/v2/w/"

// checkOutput checks the bucket holds one entry per pack and nothing else,
// that every block in a pack hash-checks against its key and keeps its refs,
// and that each block of want sits in exactly one pack. It returns the pack
// ids.
func checkOutput(t *testing.T, b *fakeBucket, src *fakeSource, want []*block.BlockRef) []string {
	t.Helper()
	packs, entries := b.keysUnder(testPrefix+"packs/"), b.keysUnder(testPrefix+"entries/")
	if !slices.Equal(packs, entries) {
		t.Fatalf("packs %v and entries %v differ", packs, entries)
	}
	seen := make(map[string]int)
	for _, id := range packs {
		data := b.objects[testPrefix+"packs/"+id]
		entry := &packfile.PackfileEntry{}
		if err := entry.UnmarshalVT(b.objects[testPrefix+"entries/"+id]); err != nil {
			t.Fatal(err)
		}
		if entry.GetId() != id || entry.GetSizeBytes() != uint64(len(data)) {
			t.Fatalf("entry %s names %s of %d bytes, pack is %d", id, entry.GetId(), entry.GetSizeBytes(), len(data))
		}
		rdr, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		var count uint64
		err = rdr.ScanPrefix(nil, func(key, value []byte) error {
			count++
			h, err := packfile.ParseBlockKey(key)
			if err != nil {
				return err
			}
			stored, err := block.DecodeBlockObject(value)
			if err != nil {
				return err
			}
			ref := block.NewBlockRef(h)
			if err := ref.VerifyData(stored.Data, false); err != nil {
				return errors.Wrap(err, "block does not hash-check against its key")
			}
			orig := src.blocks[ref.MarshalString()]
			if !stored.RefsKnown || len(stored.Refs) != len(orig.Refs) {
				return errors.Errorf("block %s lost its refs", ref.MarshalString())
			}
			seen[ref.MarshalString()]++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if count != entry.GetBlockCount() {
			t.Fatalf("pack %s holds %d blocks, entry says %d", id, count, entry.GetBlockCount())
		}
	}
	for _, ref := range want {
		if seen[ref.MarshalString()] != 1 {
			t.Fatalf("block %s in %d packs, want 1", ref.MarshalString(), seen[ref.MarshalString()])
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("packs hold %d blocks, want %d", len(seen), len(want))
	}
	if b.deletes != 0 || b.overwrites != 0 {
		t.Fatalf("bucket saw %d deletes and %d overwrites", b.deletes, b.overwrites)
	}
	return packs
}

// TestFetch packs every listed block once, hash-checked, in entries a store
// can adopt, and lists the blocks no source holds.
func TestFetch(t *testing.T) {
	ctx := t.Context()
	src, refs := newBlocks(t, 50)
	missing, _ := block.BuildBlockRef([]byte("a block no source holds"), &block.PutOpts{})
	bkt := newFakeBucket()
	var adopted []string
	f := &Fetcher{Source: src, Bucket: bkt, BucketName: "b", Prefix: testPrefix, BatchBlocks: 8, Reads: 4,
		OnPack: func(e *packfile.PackfileEntry) { adopted = append(adopted, e.GetId()) }}
	// List each block twice and the missing one, out of order.
	list := append(slices.Clone(refs), refs...)
	list = append(list, missing)
	slices.Reverse(list)
	res, err := f.Run(ctx, list)
	if err != nil {
		t.Fatal(err)
	}
	packs := checkOutput(t, bkt, src, refs)
	if len(res.Holes) != 1 || !res.Holes[0].EqualsRef(missing) {
		t.Fatalf("holes %v, want the missing block", res.Holes)
	}
	slices.Sort(adopted)
	if !slices.Equal(adopted, packs) {
		t.Fatalf("OnPack named %v, want %v", adopted, packs)
	}

	// A second run over the same list skips every batch and writes nothing.
	puts := bkt.puts
	res, err = f.Run(ctx, list)
	if err != nil {
		t.Fatal(err)
	}
	if bkt.puts != puts || res.Packed != 0 || res.Skipped == 0 {
		t.Fatalf("rerun wrote %d objects, packed %d, skipped %d", bkt.puts-puts, res.Packed, res.Skipped)
	}
}

// TestFetchResume kills a run at each point of each batch, by failing the
// source's reads or the bucket's puts, then resumes it, and checks the result
// is the packs a clean run writes: each block once, no orphan pack, no delete
// and no overwrite.
func TestFetchResume(t *testing.T) {
	ctx := t.Context()
	for killAt := 1; killAt < 15; killAt++ { // 5 batches of 3 puts, 30 reads
		for _, kill := range []string{"put", "read"} {
			t.Run(fmt.Sprintf("%s-%d", kill, killAt), func(t *testing.T) {
				src, refs := newBlocks(t, 30)
				bkt := newFakeBucket()
				f := &Fetcher{Source: src, Bucket: bkt, BucketName: "b", Prefix: testPrefix, BatchBlocks: 7, Reads: 1}
				if kill == "put" {
					bkt.failPutsAfter = killAt
				} else {
					src.failAfter = killAt * 2
				}
				if _, err := f.Run(ctx, refs); !errors.Is(err, errKilled) {
					t.Fatalf("killed run returned %v", err)
				}

				// Resume with a fresh fetcher, as a restart would.
				bkt.failPutsAfter, src.failAfter = 0, 0
				f = &Fetcher{Source: src, Bucket: bkt, BucketName: "b", Prefix: testPrefix, BatchBlocks: 7, Reads: 1}
				if _, err := f.Run(ctx, refs); err != nil {
					t.Fatal(err)
				}
				packs := checkOutput(t, bkt, src, refs)
				if len(packs) != 5 {
					t.Fatalf("%d packs, want 5", len(packs))
				}
			})
		}
	}
}

// TestFetchRefusesChangedPack stops rather than overwrite a pack whose bytes
// differ from what the resumed batch builds.
func TestFetchRefusesChangedPack(t *testing.T) {
	ctx := t.Context()
	src, refs := newBlocks(t, 5)
	bkt := newFakeBucket()
	bkt.failPutsAfter = 2 // the marker and the pack land, the entry does not
	f := &Fetcher{Source: src, Bucket: bkt, BucketName: "b", Prefix: testPrefix, BatchBlocks: 10, Reads: 1}
	if _, err := f.Run(ctx, refs); !errors.Is(err, errKilled) {
		t.Fatalf("killed run returned %v", err)
	}
	ids := bkt.keysUnder(testPrefix + "packs/")
	if len(ids) != 1 {
		t.Fatalf("packs %v, want one", ids)
	}
	bkt.objects[testPrefix+"packs/"+ids[0]] = []byte("other bytes")
	bkt.failPutsAfter = 0
	if _, err := f.Run(ctx, refs); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("resume over a changed pack returned %v", err)
	}
	if bkt.overwrites != 0 || bkt.deletes != 0 {
		t.Fatalf("bucket saw %d overwrites and %d deletes", bkt.overwrites, bkt.deletes)
	}
}
