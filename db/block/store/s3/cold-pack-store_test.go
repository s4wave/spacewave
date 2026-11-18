//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// TestColdPackBloomCollision forces a positive for a key that no tail holds.
func TestColdPackBloomCollision(t *testing.T) {
	// Use a real exported pack that straddles one missing SHA-256 key.
	values := coldValues(t, 3)
	f := newColdFixture(t)
	f.addPack(t, []coldValue{values[0], values[2]}, true)
	f.manifest(t)
	missing := values[1].ref

	// Occupy the background fill lane before the existence probes. They must
	// return while the bucket is stalled, with only one asynchronous request.
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f.initBucket(t)
	reader := f.reader
	f.reader = &coldTestReader{client: reader, before: func(ctx context.Context, _ string, _ int64, _ int) {
		once.Do(func() { close(started) })
		select {
		case <-release:
		case <-ctx.Done():
		}
	}}
	s := f.open(t, nil)
	if found, err := s.GetBlockExists(t.Context(), missing); found || err != nil {
		t.Fatalf("uncached positive = %v, %v", found, err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("uncached existence did not queue a background fill")
	}
	if found, err := s.GetBlockExistsBatch(t.Context(), []*block.BlockRef{missing, values[0].ref, missing}); err != nil || !slices.Equal(found, []bool{false, false, false}) {
		t.Fatalf("existence batch while fill stalled = %v, %v", found, err)
	}
	if n := f.reader.calls.Load(); n != 1 || s.Stats().UncachedExistenceMisses != 4 {
		t.Fatalf("bucket calls/uncached misses = %d/%d; wanted one async call, four misses", n, s.Stats().UncachedExistenceMisses)
	}
	close(release)
	waitCold(t, func() bool { return s.tails.get(0) != nil })

	// Every API rejects the all-ones bloom once an exact tail disproves it.
	assertColdAbsent(t, s, missing)
	if stats := s.Stats(); stats.FalsePositives != 1 || stats.TailFetches != 1 || stats.TrueHits != 0 {
		t.Fatalf("bloom collision or negative cache: %+v", stats)
	}
	if yes, err := s.GetBlockExists(t.Context(), values[0].ref); err != nil || !yes || f.reader.calls.Load() != 1 {
		t.Fatalf("resident exact existence = %v, %v; calls=%d", yes, err, f.reader.calls.Load())
	}

	// A positive through the resident index needs neither allocation nor I/O.
	ctx := t.Context()
	allocs := testing.AllocsPerRun(100, func() {
		if yes, err := s.GetBlockExists(ctx, values[0].ref); err != nil || !yes {
			t.Fatalf("warm probe = %t, %v", yes, err)
		}
	})
	if allocs != 0 || f.reader.calls.Load() != 1 {
		t.Fatalf("warm probe allocations = %g, total ranges = %d", allocs, f.reader.calls.Load())
	}
}

// TestColdPackRejectsCorruption exercises the verified bytes and gone fences.
func TestColdPackRejectsCorruption(t *testing.T) {
	// Damage on the local mirror must stop publication, even with an intact tail.
	t.Run("corrupt-mirror", func(t *testing.T) {
		// Build a writer pack, then flip its only block's data without touching the index.
		v := coldValues(t, 1)[0]
		f := newColdFixture(t)
		key := f.addPack(t, []coldValue{v}, false)
		raw := []byte(f.objects[key])
		i := bytes.Index(raw, v.data)
		if i < 0 {
			t.Fatal("fixture does not contain its payload")
		}
		raw[i] ^= 1
		if err := os.WriteFile(filepath.Join(f.root, key[len(f.opts.Prefix):]), raw, 0o600); err != nil {
			t.Fatal(err)
		}

		// The exact key stream is intact; only payload verification can reject it.
		manifest, err := BuildColdPackManifest(t.Context(), f.root, coldTestKeys(f.keys))
		if manifest != nil || !errors.Is(err, block.ErrBlockCorrupt) {
			t.Fatalf("corrupt mirror produced a manifest: %t, %v", manifest != nil, err)
		}
	})

	// A new length fences a pack even if the old range and index still match.
	t.Run("bucket-length", func(t *testing.T) {
		// Append bytes after the good mirror's authenticated index.
		v := coldValues(t, 1)[0]
		f := newColdFixture(t)
		key := f.addPack(t, []coldValue{v}, false)
		m := f.manifest(t)
		want := sha256.Sum256([]byte(f.objects[key]))
		if !bytes.Equal(m.Entries[0].PackSha256, want[:]) {
			t.Fatal("manifest did not hash the exact pack bytes")
		}
		f.objects[key] += "extra"
		s := f.open(t, nil)
		assertColdAbsent(t, s, v.ref)
		if !s.catalog.Load().packs[0].wrongObject.Load() || f.reader.calls.Load() != 1 {
			t.Fatal("wrong length did not fence the pack after its first range")
		}
	})

	// A tail without whole-object evidence cannot make a put discard good bytes.
	t.Run("unbound-tail", func(t *testing.T) {
		// Use a payload larger than the first index range, on a server with no checksum.
		data := bytes.Repeat([]byte("unbound cold body"), 1024)
		ref, err := block.BuildBlockRef(data, &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		f := newColdFixture(t)
		key := f.addPack(t, []coldValue{{ref: ref, data: data}}, false)
		f.manifest(t)
		corrupt := []byte(f.objects[key])
		i := bytes.Index(corrupt, data)
		if i < 0 {
			t.Fatal("fixture does not contain its payload")
		}
		corrupt[i] ^= 1
		f.objects[key] = string(corrupt)
		s := f.open(t, nil)
		if yes, err := s.GetBlockExists(t.Context(), ref); err != nil || yes {
			t.Fatalf("uncached extent: %t, %v", yes, err)
		}
		waitCold(t, func() bool { return s.tails.get(0) != nil })
		if yes, err := s.GetBlockExists(t.Context(), ref); err != nil || yes || f.reader.calls.Load() != 1 {
			t.Fatalf("authenticated tail with unreadable payload: %t, %v (%d ranges)", yes, err, f.reader.calls.Load())
		}
		assertColdAbsent(t, s, ref)
	})
	// A modified payload can decode, but its SHA-256 cannot match the ref.
	t.Run("payload-and-tail", func(t *testing.T) {
		// Authenticate the correct tail before corrupting only an encoded payload.
		v := coldValues(t, 1)[0]
		f := newColdFixture(t)
		key := f.addPack(t, []coldValue{v}, false)
		manifest := f.manifest(t)
		base := len(f.objects[key]) - int(manifest.Entries[0].IndexTailBytes)
		mutated := []byte(f.objects[key])
		mutated[base-1] ^= 1
		f.objects[key] = string(mutated)
		s := f.open(t, nil)
		if stored, err := s.GetStoredBlock(t.Context(), v.ref); stored != nil || err != nil {
			t.Fatalf("corrupted bytes escaped = %v, %v", stored, err)
		}
		if stat, err := s.StatBlock(t.Context(), v.ref); stat != nil || err != nil || s.Stats().PromotedKeys != 0 {
			t.Fatalf("corrupt payload stat = %v, %v; stats %+v", stat, err, s.Stats())
		}
		if yes, err := s.GetBlockExists(t.Context(), v.ref); yes || err != nil {
			t.Fatalf("cached tail vouched for disproved payload: %v, %v", yes, err)
		}
		s.Close()

		// Mutate an index key independently; it must not authorize presence.
		mutated[base+4] ^= 1
		f.setObject(key, string(mutated))
		s = f.open(t, nil)
		assertColdAbsent(t, s, v.ref)
		if s.tails.get(0) != nil {
			t.Fatal("unauthenticated tail entered the cache")
		}
	})

	// A 404 removes both forms of cached evidence for that pack and leaves
	// the other packs readable. No writer or delete method touches either prefix.
	t.Run("gone-pack", func(t *testing.T) {
		// Promote one key and keep another in the same resident tail.
		values := coldValues(t, 3)
		f := newColdFixture(t)
		key := f.addPack(t, values[:2], false)
		f.addPack(t, values[2:], false)
		manifest := f.manifest(t)
		s := f.open(t, nil)
		assertColdBlock(t, s, values[0])
		waitCold(t, func() bool { return s.Stats().PromotedKeys == 1 })
		for _, v := range values[:2] {
			if yes, err := s.GetBlockExists(t.Context(), v.ref); !yes || err != nil {
				t.Fatalf("precondition: expected exact evidence, got %v, %v", yes, err)
			}
		}

		// Observe a bucket 404, then check every positive path including stat.
		f.setObject(key, "")
		assertColdAbsent(t, s, values[0].ref)
		assertColdAbsent(t, s, values[1].ref)
		stats := s.Stats()
		if stats.LoadedPacks != 2 || !slices.Equal(stats.GonePackIDs, []string{manifest.Entries[0].Entry.Id}) {
			t.Fatalf("gone pack readback = %+v", stats)
		}
		assertColdBlock(t, s, values[2])

		// Cold only has range capability; a hot-prefix sweep cannot reach it.
		if _, _, err := s.PutBlock(t.Context(), values[2].data, nil); !errors.Is(err, ErrPackImmutable) {
			t.Fatalf("cold put: %v", err)
		}
		if _, err := s.PutBlockBatch(t.Context(), nil); !errors.Is(err, ErrPackImmutable) {
			t.Fatalf("cold put batch: %v", err)
		}
		if err := s.RmBlock(t.Context(), values[2].ref); !errors.Is(err, ErrPackImmutable) {
			t.Fatalf("cold remove: %v", err)
		}
		f.setObject("hot/"+packDir+"old", "hot")
		if err := DeletePackStore(t.Context(), f.client, "bucket", "hot/"); err != nil {
			t.Fatal(err)
		}
		assertColdBlock(t, s, values[2])
	})
}

// TestColdPackOverlappingIntervals checks inclusive endpoints and overlap.
func TestColdPackOverlappingIntervals(t *testing.T) {
	// The later-starting interval is considered first and has a forced false
	// positive for the inner key of the earlier-starting interval.
	v := coldValues(t, 8)
	f := newColdFixture(t)
	f.addPack(t, []coldValue{v[0], v[3], v[5]}, false)
	f.addPack(t, []coldValue{v[2], v[4]}, true)
	f.addPack(t, v[6:8], false)
	f.manifest(t)
	s := f.open(t, nil)
	assertColdBlock(t, s, v[3])
	if n := s.Stats().FalsePositives; n != 1 {
		t.Fatalf("overlap did not traverse the first tail miss; false positives = %d", n)
	}

	// A repeat range counts once, apart from the first read or any tail GETs.
	if got := s.Stats().RepeatExtentReads; got != 0 {
		t.Fatalf("first body read counted as repeated: %d", got)
	}
	before := f.reader.calls.Load()
	assertColdBlock(t, s, v[3])
	if got := s.Stats().RepeatExtentReads; got != 1 || f.reader.calls.Load() != before+1 {
		t.Fatalf("repeated body ranges = %d, requests = %d", got, f.reader.calls.Load()-before)
	}

	// The exact first and last key of each pack remain addressable.
	for _, i := range []int{0, 2, 4, 5, 6, 7} {
		assertColdBlock(t, s, v[i])
	}
}

// TestColdPackUnresolvedExistence keeps a good body reachable after another
// extent in its pack has failed verification and a second tail misses the key.
func TestColdPackUnresolvedExistence(t *testing.T) {
	// Mount two overlapping packs; the second has an all-ones bloom.
	v := coldValues(t, 5)
	f := newColdFixture(t)
	key := f.addPack(t, []coldValue{v[0], v[2], v[4]}, false)
	f.addPack(t, []coldValue{v[1], v[3]}, true)
	f.manifest(t)
	s := f.open(t, nil)
	assertColdBlock(t, s, v[3])
	assertColdBlock(t, s, v[0])

	// Disprove one known extent after caching the original tails.
	raw := []byte(f.objects[key])
	i := bytes.Index(raw, v[4].data)
	if i < 0 {
		t.Fatal("fixture does not contain its payload")
	}
	raw[i] ^= 1
	f.setObject(key, string(raw))
	if stored, err := s.GetStoredBlock(t.Context(), v[4].ref); stored != nil || err != nil {
		t.Fatalf("bad body = %v, %v", stored, err)
	}
	if !s.catalog.Load().packs[0].badPayload.Load() {
		t.Fatal("failed body did not fence existence")
	}

	// Existence skips the fenced pack; its body can still verify a good key.
	if yes, err := s.GetBlockExists(t.Context(), v[2].ref); yes || err != nil {
		t.Fatalf("fenced existence = %t, %v", yes, err)
	}
	assertColdBlock(t, s, v[2])
}

// TestColdPackFlightCancellation checks that the first waiter's cancellation
// does not cancel a demand tail or payload GET that a second waiter joined.
func TestColdPackFlightCancellation(t *testing.T) {
	for _, lane := range []string{"tail", "extent"} {
		t.Run(lane, func(t *testing.T) {
			// Gate one fake-bucket request until both callers are waiting.
			v := coldValues(t, 1)[0]
			f := newColdFixture(t)
			f.addPack(t, []coldValue{v}, false)
			m := f.manifest(t)
			f.initBucket(t)
			started, release, joined := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			f.reader.before = func(ctx context.Context, _ string, _ int64, _ int) {
				once.Do(func() { close(started) })
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			s := f.open(t, nil)
			catalog := s.catalog.Load()
			extent := coldExtent{size: m.Entries[0].Entry.SizeBytes - m.Entries[0].IndexTailBytes}
			read := func(ctx context.Context) error {
				if lane == "tail" {
					_, err := s.tails.read(ctx, catalog, 0, false)
					return err
				}
				_, err := s.readExtent(ctx, catalog, v.ref, extent, false)
				return err
			}

			// Join before canceling the leader and release only after it returns.
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			leader, stop := context.WithCancel(ctx)
			defer stop()
			first, second := make(chan error, 1), make(chan error, 1)
			go func() { first <- read(leader) }()
			awaitColdSignal(t, ctx, started)
			go func() { second <- read(&coldJoinContext{Context: ctx, joined: joined}) }()
			awaitColdSignal(t, ctx, joined)
			stop()
			if err := awaitColdRead(t, ctx, first); !errors.Is(err, context.Canceled) {
				t.Fatalf("leader = %v, want cancellation", err)
			}
			close(release)
			if err := awaitColdRead(t, ctx, second); err != nil || f.reader.calls.Load() != 1 {
				t.Fatalf("joined read = %v, ranges = %d", err, f.reader.calls.Load())
			}
		})
	}
}

// coldJoinContext observes the select after the waiter joined the flight.
type coldJoinContext struct {
	context.Context
	// joined closes when the waiter first asks for its cancellation channel.
	joined chan struct{}
}

// Done marks when the caller reached the wait, not when a goroutine started.
func (c *coldJoinContext) Done() <-chan struct{} {
	close(c.joined)
	return c.Context.Done()
}

// awaitColdSignal bounds the wait for a fixture milestone.
func awaitColdSignal(t *testing.T, ctx context.Context, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatal("cold flight did not reach the expected state")
	}
}

// awaitColdRead bounds the wait for a flight caller to return.
func awaitColdRead(t *testing.T, ctx context.Context, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		t.Fatal("cold flight caller did not return")
		return ctx.Err()
	}
}

// TestColdPackPromotionRecovery keeps the valid prefix after a crash.
func TestColdPackPromotionRecovery(t *testing.T) {
	for _, kind := range []string{"partial", "bad-crc"} {
		t.Run(kind, func(t *testing.T) {
			// Commit the first key before simulating a torn append of the second.
			v := coldValues(t, 2)
			f := newColdFixture(t)
			key := f.addPack(t, v, false)
			manifest := f.manifest(t)
			s := f.open(t, nil)
			assertColdBlock(t, s, v[0])
			waitCold(t, func() bool { return s.Stats().PromotedKeys == 1 })
			s.Close()
			f.appendTear(t, key, manifest.Entries[0], v[1], kind)

			// Keep warming out of this branch to measure an exact promoted GET.
			s = f.open(t, func(opts *ColdPackOptions) { opts.TailCacheBytes = 1 })
			checkColdLog(t, f.opts.PromotedLog, 1)
			before := f.reader.calls.Load()
			if yes, err := s.GetBlockExists(t.Context(), v[0].ref); yes || err != nil {
				t.Fatalf("restored suffix alone said present: %v, %v", yes, err)
			}
			collisionHash := v[0].ref.GetHash().Clone()
			collisionHash.Hash[0] ^= 1
			collision := block.NewBlockRef(collisionHash)
			if yes, err := s.GetBlockExists(t.Context(), collision); yes || err != nil || f.reader.calls.Load() != before {
				t.Fatalf("suffix collision answered %v, %v with %d reads", yes, err, f.reader.calls.Load()-before)
			}
			assertColdBlock(t, s, v[0])
			if s.Stats().TailFetches != 0 || f.reader.calls.Load() != before+1 {
				t.Fatalf("restored extent did not cost one exact read: %+v", s.Stats())
			}
			if yes, _ := s.GetBlockExists(t.Context(), collision); yes {
				t.Fatal("full proof authorized different key with same suffix")
			}
			if yes, _ := s.GetBlockExists(t.Context(), v[0].ref); !yes {
				t.Fatal("verified promoted identity did not answer from RAM")
			}

			// Stat uses the promoted decoded size without another GET.
			stat, err := s.StatBlock(t.Context(), v[0].ref)
			if err != nil || stat == nil || stat.Size != int64(len(v[0].data)) || f.reader.calls.Load() != before+1 {
				t.Fatalf("promoted stat = %v, %v; requests=%d", stat, err, f.reader.calls.Load()-before)
			}

			// The torn key can be found through its tail and appended again.
			assertColdBlock(t, s, v[1])
			waitCold(t, func() bool { return s.Stats().PromotedKeys == 2 })
			checkColdLog(t, f.opts.PromotedLog, 2)
			s.Close()

			// Restart with a sufficient cache and warm one unique pack in the background.
			s = f.open(t, nil)
			waitCold(t, func() bool { return s.tails.get(0) != nil })
			if s.Stats().TailFetches != 1 {
				t.Fatalf("recovered records did not coalesce startup warming: %+v", s.Stats())
			}
		})
	}
}

// TestColdPackPromotionBackpressure proves a paused writer cannot stall a read.
func TestColdPackPromotionBackpressure(t *testing.T) {
	// Fill a healthy writer's queue without running its drain, simulating an
	// fsync stall deterministically instead of relying on the host's disk speed.
	v := coldValues(t, 2)
	f := newColdFixture(t)
	f.addPack(t, v, false)
	f.manifest(t)
	s := f.open(t, func(opts *ColdPackOptions) { opts.PromotedLog = "" })
	s.promotions.active.Store(true)
	for range cap(s.promotions.promoteCh) {
		s.promotions.promoteCh <- coldPromotion{}
	}

	// Hold the promoter's publication lock as well as its full queue; readers
	// still return a verified block promptly.
	s.promotions.mtx.Lock()
	const readers = 32
	results := make(chan bool, readers)
	for range readers {
		go func() {
			data, found, err := s.GetBlock(t.Context(), v[0].ref)
			results <- found && err == nil && bytes.Equal(data, v[0].data)
		}()
	}
	for range readers {
		select {
		case ok := <-results:
			if !ok {
				t.Fatal("backpressure hid or changed a cold block")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("a reader waited on the stopped promoter")
		}
	}
	s.promotions.mtx.Unlock()
	if s.Stats().PromoteDropped != readers {
		t.Fatalf("queue overflows not counted: %+v", s.Stats())
	}
	s.Close()

	// A real writer stops exactly at the configured physical record cap.
	s = f.open(t, func(opts *ColdPackOptions) { opts.PromoteCap = 1 })
	assertColdBlock(t, s, v[0])
	waitCold(t, func() bool { return s.Stats().PromotedKeys == 1 })
	assertColdBlock(t, s, v[1])
	checkColdLog(t, f.opts.PromotedLog, 1)
	if s.Stats().PromoteCapDropped != 1 {
		t.Fatalf("physical cap overflows not counted: %+v", s.Stats())
	}
}

// coldValue holds one test's data and canonical packed ref.
type coldValue struct {
	// ref names the SHA-256 of data.
	ref *block.BlockRef
	// data is the stable plaintext bytes expected from cold reads.
	data []byte
}

// coldValues supplies deterministic values sorted in pack index order.
func coldValues(t *testing.T, n int) []coldValue {
	// Make distinct, verified hashes without assuming the order of their inputs.
	t.Helper()
	values := make([]coldValue, n)
	for i := range values {
		data := []byte("cold test block " + strconv.Itoa(i))
		h, err := hash.Sum(hash.RecommendedHashType, data)
		if err != nil {
			t.Fatal(err)
		}
		values[i] = coldValue{ref: block.NewBlockRef(h), data: data}
	}
	slices.SortFunc(values, func(a, b coldValue) int { return bytes.Compare(a.ref.GetHash().GetHash(), b.ref.GetHash().GetHash()) })
	return values
}

// coldFixture mirrors the writer's exact pack and entry objects into a fake bucket.
type coldFixture struct {
	// root holds the offline files used to build the manifest.
	root string
	// objects holds the initial key/value set for the fake bucket.
	objects map[string]string
	// keys contains the exported canonical keys for this small test.
	keys [][]byte
	// parts numbers the fixture's export parts.
	parts int
	// opts describes the cold mount and writable test log.
	opts ColdPackOptions
	// bucket exposes the real in-memory HTTP S3 fixture.
	bucket *fakeBucket
	// client is used to test a hot-prefix deletion against the same bucket.
	client *Client
	// reader counts calls and can stall a ranged GET before it hits the bucket.
	reader *coldTestReader
}

// newColdFixture creates an isolated exported pack mirror.
func newColdFixture(t *testing.T) *coldFixture {
	// Leave the manifest and log outside the directory the builder walks.
	t.Helper()
	work := t.TempDir()
	return &coldFixture{root: filepath.Join(work, "mirror"), objects: make(map[string]string), opts: ColdPackOptions{
		Prefix: "cold/", ManifestFile: filepath.Join(work, "manifest"), PromotedLog: filepath.Join(work, "promoted"), PromoteCap: 64,
	}}
}

// addPack writes one part with the native pack writer and object layout.
func (f *coldFixture) addPack(t *testing.T, values []coldValue, falsePositive bool) string {
	// Build a real kvfile and collect the independent export's keys.
	t.Helper()
	var buf bytes.Buffer
	i := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if i == len(values) {
			return nil, nil, nil
		}
		v := values[i]
		i++
		f.keys = append(f.keys, packfile.BlockKey(v.ref.GetHash()))
		return v.ref.GetHash(), &block.StoredBlock{Data: v.data, RefsKnown: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.BuildPackID("cold-test", result)
	if err != nil {
		t.Fatal(err)
	}
	entry := &packfile.PackfileEntry{Id: id, BlockCount: result.BlockCount, SizeBytes: result.BytesWritten, BloomFilter: result.BloomFilter, BloomFormatVersion: packfile.BloomFormatVersionV1}

	// Every absent key becomes a bloom hit only when the test asks for it.
	if falsePositive {
		entry.BloomFilter, err = bloom.NewBloom(bloom.From([]uint64{^uint64(0)}, 64, 7)).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
	}
	encoded, err := entry.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Mirror the standard writer's object names, without a file extension.
	part := "X" + strconv.Itoa(f.parts) + "/"
	f.parts++
	for name, raw := range map[string][]byte{packDir + id: buf.Bytes(), entryDir + id: encoded} {
		path := filepath.Join(f.root, part, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		f.objects[f.opts.Prefix+part+name] = string(raw)
	}
	return f.opts.Prefix + part + packDir + id
}

// manifest builds and saves the same protobuf the CLI publishes.
func (f *coldFixture) manifest(t *testing.T) *packfile.ColdPackManifest {
	// Sort the independent key stream and run the production verifier.
	t.Helper()
	slices.SortFunc(f.keys, bytes.Compare)
	manifest, err := BuildColdPackManifest(t.Context(), f.root, coldTestKeys(f.keys))
	if err != nil {
		t.Fatal(err)
	}
	data, err := manifest.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.opts.ManifestFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return manifest
}

// open starts a real store and waits only in the test for its catalog to publish.
func (f *coldFixture) open(t *testing.T, configure func(*ColdPackOptions)) *ColdPackStore {
	// Reuse the same bucket on restart; copy options before overriding them.
	t.Helper()
	f.initBucket(t)
	opts := f.opts
	if configure != nil {
		configure(&opts)
	}
	s, err := NewColdPackStore(t.Context(), nil, f.reader, "bucket", opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	waitCold(t, func() bool { return s.Stats().Ready })
	return s
}

// initBucket starts the HTTP fixture once so normal S3 status handling is tested.
func (f *coldFixture) initBucket(t *testing.T) {
	// Preserve any deliberately configured gate around the real client.
	t.Helper()
	if f.bucket != nil {
		return
	}
	f.bucket, f.client = newFakeBucket(t, f.objects)
	f.reader = &coldTestReader{client: f.client}
}

// setObject replaces bytes in the test-only bucket, or hides an object.
func (f *coldFixture) setObject(key, body string) {
	// The owner of the fake bucket protects live HTTP readers.
	f.bucket.mtx.Lock()
	defer f.bucket.mtx.Unlock()
	if body == "" {
		delete(f.bucket.objects, key)
		return
	}
	f.bucket.objects[key] = body
}

// appendTear writes an incomplete or bad-CRC record for v after a durable prefix.
func (f *coldFixture) appendTear(t *testing.T, key string, meta *packfile.ColdPackEntry, v coldValue, kind string) {
	// Use the real extent so a valid-looking partial record is understood.
	t.Helper()
	body := []byte(f.objects[key])
	r, err := kvfile.BuildReader(bytes.NewReader(body), uint64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	e, _, err := r.SearchIndexEntryWithKey(packfile.BlockKey(v.ref.GetHash()))
	if e == nil || err != nil || meta.Entry.SizeBytes != uint64(len(body)) {
		t.Fatalf("find torn key in fixture: %v, %v", e, err)
	}
	digest := [32]byte(v.ref.GetHash().GetHash())
	record := encodeColdPromotion(nil, coldPromotion{extent: coldExtent{off: e.Offset, size: e.Size}, digest: digest, dataSize: uint32(len(v.data))})
	if kind == "partial" {
		record = record[:15]
	} else {
		record[len(record)-1] ^= 1
	}

	// Simulate bytes left by a crashed append with no mounted writer.
	file, err := os.OpenFile(f.opts.PromotedLog, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(record); err != nil {
		t.Error(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// coldTestReader counts and optionally gates real range requests.
type coldTestReader struct {
	// client performs the actual HTTP read.
	client ColdPackRangeReader
	// before runs before issuing a range request in a test.
	before func(context.Context, string, int64, int)
	// calls counts range requests admitted by the tested store.
	calls atomic.Uint64
}

// GetColdPackRange exposes a gate before calling the real reader.
func (r *coldTestReader) GetColdPackRange(ctx context.Context, bucket, key string, off int64, size int, total uint64) ([]byte, []byte, error) {
	// Count attempted requests even while their network is stalled.
	r.calls.Add(1)
	if r.before != nil {
		r.before(ctx, key, off, size)
	}
	return r.client.GetColdPackRange(ctx, bucket, key, off, size, total)
}

// coldTestKeys exposes the fixture key list as an iterator.
func coldTestKeys(keys [][]byte) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for _, key := range keys {
			if !yield(key, nil) {
				return
			}
		}
	}
}

// waitCold bounds test-only waiting for a background transition.
func waitCold(t *testing.T, ready func() bool) {
	// Poll a counter or immutable publication, never a filesystem write.
	t.Helper()
	deadline, tick := time.NewTimer(2*time.Second), time.NewTicker(time.Millisecond)
	defer deadline.Stop()
	defer tick.Stop()
	for !ready() {
		select {
		case <-deadline.C:
			t.Fatal("cold background transition did not complete")
		case <-tick.C:
		}
	}
}

// assertColdAbsent checks all read operations agree that a ref is not held.
func assertColdAbsent(t *testing.T, s *ColdPackStore, ref *block.BlockRef) {
	// Exercise body, metadata and put-deduplication paths for the same ref.
	t.Helper()
	ctx := t.Context()
	if data, ok, err := s.GetBlock(ctx, ref); data != nil || ok || err != nil {
		t.Errorf("GetBlock = %x, %v, %v", data, ok, err)
	}
	if b, err := s.GetStoredBlock(ctx, ref); b != nil || err != nil {
		t.Errorf("GetStoredBlock = %v, %v", b, err)
	}
	if ok, err := s.GetBlockExists(ctx, ref); ok || err != nil {
		t.Errorf("GetBlockExists = %v, %v", ok, err)
	}
	if found, err := s.GetBlockExistsBatch(ctx, []*block.BlockRef{ref, ref}); err != nil || !slices.Equal(found, []bool{false, false}) {
		t.Errorf("GetBlockExistsBatch = %v, %v", found, err)
	}
	if stat, err := s.StatBlock(ctx, ref); stat != nil || err != nil {
		t.Errorf("StatBlock = %v, %v", stat, err)
	}
}

// assertColdBlock checks the bytes and refs from the real decoder.
func assertColdBlock(t *testing.T, s *ColdPackStore, v coldValue) {
	// Reject a data-only or unverified read of the written leaf.
	t.Helper()
	stored, err := s.GetStoredBlock(t.Context(), v.ref)
	if err != nil || !bytes.Equal(stored.GetData(), v.data) || !stored.GetRefsKnown() || len(stored.GetRefs()) != 0 {
		t.Fatalf("cold block = %v, %v; wanted leaf %q", stored, err, v.data)
	}
}

// checkColdLog checks the disk footprint, not just the number of map entries.
func checkColdLog(t *testing.T, path string, records int64) {
	// A partial or oversized file must not survive a completed recovery or append.
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != records*coldPromotionBytes {
		t.Fatalf("promoted log size %d; wanted %d", info.Size(), records*coldPromotionBytes)
	}
}

// _ is a type assertion.
var _ ColdPackRangeReader = (*coldTestReader)(nil)
