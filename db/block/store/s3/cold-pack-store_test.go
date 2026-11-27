//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash/crc32"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
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

		// The exact key stream is intact, so only payload verification
		// can reject it.
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

		// Mutate an index key independently. The key must not authorize
		// presence.
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

		// Cold only has range capability, so a hot-prefix sweep cannot
		// reach it.
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
	// Mount two overlapping packs. The second has an all-ones bloom.
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

	// Existence skips the fenced pack, but its body can still verify a good
	// key.
	if yes, err := s.GetBlockExists(t.Context(), v[2].ref); yes || err != nil {
		t.Fatalf("fenced existence = %t, %v", yes, err)
	}
	assertColdBlock(t, s, v[2])
}

// TestColdPackMoveIsolation keeps two move ranges in flight and two queued
// while foreground readers take the remaining slots or fail fast.
func TestColdPackMoveIsolation(t *testing.T) {
	// Use real writer bytes and a gated in-memory range reader so the scheduler
	// can prove that a caller returned without any timer or range being released.
	v := coldValues(t, 11)
	f := newColdFixture(t)
	key := f.addPack(t, v, false)
	manifest := f.manifest(t)
	raw := []byte(f.objects[key])

	// Park every body request and advance only when all waiters are blocked.
	synctest.Test(t, func(t *testing.T) {
		// Load the tail before gating body I/O. Nothing else reads the
		// bucket.
		ctx := t.Context()
		release := make(chan struct{})
		defer close(release)
		var moves, users atomic.Int32
		reader := coldRangeProbe(func(ctx context.Context, off int64, size int) ([]byte, error) {
			// A tail GET for this small fixture covers the whole object.
			if size != len(raw) {
				if ctx.Value(coldMoveProbeKey{}) != nil {
					moves.Add(1)
				} else {
					users.Add(1)
				}
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return raw[off : off+int64(size)], nil
		})
		opts := f.opts
		opts.PromotedLog = ""
		s, err := NewColdPackStore(ctx, nil, reader, "bucket", opts)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Close)
		synctest.Wait()
		catalog := s.catalog.Load()
		if catalog == nil || len(manifest.Entries) != len(catalog.packs) {
			t.Fatal("fixture manifest did not load")
		}
		if _, err := s.tails.read(ctx, catalog, 0, false); err != nil {
			t.Fatal(err)
		}

		// Start four independent moves and verify only two enter the bucket.
		moveCtx := context.WithValue(ctx, coldMoveProbeKey{}, true)
		cancelCtx, cancel := context.WithCancel(moveCtx)
		defer cancel()
		movesDone := make(chan error, 4)
		for i, value := range v[:4] {
			go func() {
				// The last caller has a cancellation independent of the move.
				ctx := moveCtx
				if i == 3 {
					ctx = cancelCtx
				}
				_, err := s.GetStoredBlockForMove(ctx, value.ref)
				movesDone <- err
			}()
		}
		synctest.Wait()
		if got := moves.Load(); got != 2 {
			t.Errorf("moving ranges in flight = %d, want 2", got)
		}

		// Six later users must reach the bucket even with movers still waiting.
		for _, value := range v[4:10] {
			go func() { _, _ = s.GetStoredBlock(ctx, value.ref) }()
		}
		synctest.Wait()
		if got := users.Load(); got != 6 {
			t.Errorf("user ranges admitted behind moves = %d, want 6", got)
		}

		// A seventh user misses immediately, not after a held slot or deadline.
		refused := make(chan bool, 1)
		start := time.Now()
		go func() {
			stored, err := s.GetStoredBlock(ctx, v[10].ref)
			refused <- stored == nil && err == nil
		}()
		synctest.Wait()
		select {
		case ok := <-refused:
			if !ok || time.Since(start) != 0 {
				t.Error("full demand lane did not report an immediate user miss")
			}
		default:
			t.Error("user waited while the mover and all ranges were held")
		}

		// A canceled move is unresolved, not a successful skip.
		cancel()
		synctest.Wait()
		select {
		case err := <-movesDone:
			if !errors.Is(err, block.ErrUnavailable) {
				t.Errorf("canceled move = %v, want unavailable", err)
			}
		default:
			t.Error("canceled move did not leave the permit wait")
		}
	})
}

// TestColdPackMoveReturnsPermits checks releases before and after range admission.
func TestColdPackMoveReturnsPermits(t *testing.T) {
	for _, fault := range []string{"cancelled-waiter", "range-error", "deadline", "panic"} {
		t.Run(fault, func(t *testing.T) {
			// Test with the production scheduler, but no network or real deadlines.
			v := coldValues(t, coldDemandSlots+1)
			f := newColdFixture(t)
			key := f.addPack(t, v, false)
			f.manifest(t)
			raw := []byte(f.objects[key])
			f.opts.PromotedLog = ""
			synctest.Test(t, func(t *testing.T) {
				checkColdPermitReturn(t, f.opts, v, raw, fault)
			})
		})
	}
}

// coldPermitGateKey carries a test-only range gate through a demand flight.
type coldPermitGateKey struct{}

// coldPermitFaultKey makes only the selected move encounter a range fault.
type coldPermitFaultKey struct{}

// coldPermitGate holds admitted calls and records which lane reached the reader.
type coldPermitGate struct {
	// entered reports one true value for a move and one false for a user range.
	entered chan bool
	// release lets the wave leave the fake reader before another wave starts.
	release chan struct{}
}

// checkColdPermitReturn saturates and refills the real lanes in one synctest bubble.
func checkColdPermitReturn(t *testing.T, opts ColdPackOptions, v []coldValue, raw []byte, fault string) {
	// Use the real tail once. All later calls exercise only body admission.
	t.Helper()
	reader := coldRangeProbe(func(ctx context.Context, off int64, size int) ([]byte, error) {
		if gate, ok := ctx.Value(coldPermitGateKey{}).(*coldPermitGate); ok {
			gate.entered <- ctx.Value(coldMoveProbeKey{}) != nil
			select {
			case <-gate.release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if ctx.Value(coldPermitFaultKey{}) != nil {
			return nil, coldPermitFault(ctx, fault)
		}
		return raw[off : off+int64(size)], nil
	})
	s, err := NewColdPackStore(t.Context(), nil, reader, "bucket", opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	synctest.Wait()
	catalog := s.catalog.Load()
	if catalog == nil {
		t.Fatal("fixture catalog did not load")
	}
	if _, err := s.tails.read(t.Context(), catalog, 0, false); err != nil {
		t.Fatal(err)
	}

	// Saturate with users, then hold a move at its second admission step.
	drain := startColdPermitWave(t, s, v[:coldDemandSlots], 0)
	waiting, cancel := context.WithCancel(t.Context())
	defer cancel()
	queued := make(chan error, 1)
	ref := v[coldDemandSlots].ref
	go func() {
		_, err := s.GetStoredBlockForMove(waiting, ref)
		queued <- err
	}()
	synctest.Wait()
	if len(s.ranges) != coldDemandSlots || len(s.moveRanges) != 1 || len(queued) != 0 {
		t.Fatal("fixture did not park a reserved move behind a full shared lane")
	}

	// Request the identical extent. A user cannot inherit the move's wait.
	start := time.Now()
	miss := make(chan bool, 1)
	go func() {
		stored, err := s.GetStoredBlock(t.Context(), ref)
		miss <- stored == nil && err == nil
	}()
	synctest.Wait()
	select {
	case ok := <-miss:
		if !ok || time.Since(start) != 0 {
			t.Error("user did not miss immediately on the queued move's extent")
		}
	default:
		t.Error("user joined a move waiting for admission")
	}

	// The canceled waiter returns its reservation without consuming a shared slot.
	cancel()
	synctest.Wait()
	select {
	case err := <-queued:
		if !errors.Is(err, block.ErrUnavailable) {
			t.Errorf("canceled waiter = %v, want unavailable", err)
		}
	default:
		t.Error("canceled waiter did not leave admission")
	}
	drain()
	assertColdPermitsEmpty(t, s)

	// A failure after admission returns both permits too, even on panic unwind.
	if fault != "cancelled-waiter" {
		checkColdAdmittedFault(t, s, ref, fault)
		assertColdPermitsEmpty(t, s)
	}
	drain = startColdPermitWave(t, s, v[:coldDemandSlots], coldMoveSlots)
	drain()
	assertColdPermitsEmpty(t, s)
}

// startColdPermitWave holds a full wave inside the reader and returns its drain.
func startColdPermitWave(t *testing.T, s *ColdPackStore, v []coldValue, movers int) func() {
	// Start all reads, giving each a distinct extent so nothing coalesces.
	t.Helper()
	gate := &coldPermitGate{entered: make(chan bool, len(v)), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(gate.release) })
	t.Cleanup(release)
	ctx := context.WithValue(t.Context(), coldPermitGateKey{}, gate)
	moveCtx := context.WithValue(ctx, coldMoveProbeKey{}, true)
	done := make(chan bool, len(v))
	for i, value := range v {
		go func() {
			var stored *block.StoredBlock
			var err error
			if i < movers {
				stored, err = s.GetStoredBlockForMove(moveCtx, value.ref)
			} else {
				stored, err = s.GetStoredBlock(ctx, value.ref)
			}
			done <- err == nil && bytes.Equal(stored.GetData(), value.data)
		}()
	}

	// Wait for the fake ranges to park without releasing a permit or a timer.
	synctest.Wait()
	if got := len(gate.entered); got != len(v) {
		t.Fatalf("fresh wave admitted %d of %d ranges", got, len(v))
	}
	var enteredMoves int
	for range v {
		if <-gate.entered {
			enteredMoves++
		}
	}
	if enteredMoves != movers {
		t.Fatalf("fresh wave admitted %d move ranges, want %d", enteredMoves, movers)
	}

	// Release only this wave and check every read returned verified data.
	return func() {
		release()
		synctest.Wait()
		for range v {
			select {
			case ok := <-done:
				if !ok {
					t.Error("admitted wave lost a verified body")
				}
			default:
				t.Fatal("released wave did not leave the reader")
			}
		}
	}
}

// coldPermitFault injects an error, an expired range context or a reader panic.
func coldPermitFault(ctx context.Context, fault string) error {
	switch fault {
	case "range-error":
		return errors.New("fake range error")
	case "deadline":
		<-ctx.Done()
		return ctx.Err()
	default:
		panic("fake range panic")
	}
}

// checkColdAdmittedFault observes failure outside the synchronous move read.
func checkColdAdmittedFault(t *testing.T, s *ColdPackStore, ref *block.BlockRef, fault string) {
	// A test recovers the fake reader's panic only after the move has unwound.
	t.Helper()
	ctx := context.WithValue(t.Context(), coldPermitFaultKey{}, true)
	var err error
	var recovered any
	start := time.Now()
	func() {
		defer func() { recovered = recover() }()
		_, err = s.GetStoredBlockForMove(ctx, ref)
	}()
	if fault == "panic" {
		if recovered != "fake range panic" {
			t.Fatalf("fake range panic = %v", recovered)
		}
		return
	}

	// A failed range never becomes a clean miss, including when its timer ends.
	if recovered != nil || !errors.Is(err, block.ErrUnavailable) {
		t.Fatalf("move fault %s = %v, panic %v; want unavailable", fault, err, recovered)
	}
	if fault == "deadline" && time.Since(start) != coldReadLimit {
		t.Fatalf("move range deadline took %v, want %v", time.Since(start), coldReadLimit)
	}
}

// assertColdPermitsEmpty checks reservations as well as the shared range lane.
func assertColdPermitsEmpty(t *testing.T, s *ColdPackStore) {
	t.Helper()
	if shared, moves := len(s.ranges), len(s.moveRanges); shared != 0 || moves != 0 {
		t.Fatalf("returned calls kept permits: shared=%d move=%d", shared, moves)
	}
}

// coldMoveProbeKey identifies a move's gated ranges without inspecting its refs.
type coldMoveProbeKey struct{}

// coldRangeProbe is an in-memory range reader for the scheduler test.
type coldRangeProbe func(context.Context, int64, int) ([]byte, error)

// GetColdPackRange returns a fixture range without an HTTP goroutine.
func (r coldRangeProbe) GetColdPackRange(ctx context.Context, _, _ string, off int64, size int, _ uint64) ([]byte, []byte, error) {
	// No whole-object proof is needed because the test reads and verifies bodies.
	data, err := r(ctx, off, size)
	return data, nil, err
}

// TestColdPackMoveKeepsUserTail holds a move after fetching a second tail.
func TestColdPackMoveKeepsUserTail(t *testing.T) {
	// The user budget fits either tail alone, but not both together.
	v := coldValues(t, 2)
	f := newColdFixture(t)
	f.addPack(t, v[:1], false)
	moveKey := f.addPack(t, v[1:], false)
	m := f.manifest(t)
	limit := max(coldTailCharge(m.Entries[0]), coldTailCharge(m.Entries[1]))
	f.initBucket(t)

	// Stall just the first moving body request, after its tail was loaded.
	started, release := make(chan struct{}), make(chan struct{})
	releaseMove := sync.OnceFunc(func() { close(release) })
	defer releaseMove()
	var gate sync.Once
	f.reader.before = func(ctx context.Context, key string, _ int64, size int) {
		if key == moveKey && size < int(m.Entries[1].Entry.SizeBytes) {
			gate.Do(func() { close(started) })
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}
	s := f.open(t, func(opts *ColdPackOptions) {
		opts.TailCacheBytes = int(limit)
		opts.PromotedLog = ""
	})
	assertColdBlock(t, s, v[0])

	// A user must still read through its old tail while the move is held.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.GetStoredBlockForMove(ctx, v[1].ref)
		done <- err
	}()
	awaitColdSignal(t, ctx, started)
	before := s.Stats().TailFetches
	assertColdBlock(t, s, v[0])
	if got := s.Stats().TailFetches - before; got != 0 {
		t.Fatalf("move evicted the user's tail: user read fetched %d more tails", got)
	}

	// Let the mover finish, then reuse its own tail without another tail GET.
	releaseMove()
	if err := awaitColdRead(t, ctx, done); err != nil {
		t.Fatal(err)
	}
	stored, err := s.GetStoredBlockForMove(ctx, v[1].ref)
	if err != nil || !stored.GetRefsSource().Recorded() || !bytes.Equal(stored.GetData(), v[1].data) || s.Stats().TailFetches != before {
		t.Fatalf("move did not reuse its verified tail: %v, %v, tails=%d", stored, err, s.Stats().TailFetches)
	}
}

// TestColdPackMoveKeepsUserTailOrder checks that borrowing cannot touch the user LRU.
func TestColdPackMoveKeepsUserTailOrder(t *testing.T) {
	// Fit two decoded tails, but not three, and leave room to admit each raw tail.
	const count = 16
	v := coldValues(t, 3*count)
	f := newColdFixture(t)
	for i := range 3 {
		f.addPack(t, v[i*count:(i+1)*count], false)
	}
	m := f.manifest(t)
	const budget = 2 * count * coldTailEntryBytes
	for _, meta := range m.Entries {
		if coldTailCharge(meta) > budget {
			t.Fatal("fixture raw tail must fit the two decoded tail budget")
		}
	}
	s := f.open(t, func(opts *ColdPackOptions) {
		opts.TailCacheBytes = budget
		opts.PromotedLog = ""
	})

	// Fill A then B, then have the mover borrow A without issuing a tail GET.
	assertColdBlock(t, s, v[0])
	assertColdBlock(t, s, v[count])
	before := s.Stats().TailFetches
	stored, err := s.GetStoredBlockForMove(t.Context(), v[0].ref)
	if err != nil || !bytes.Equal(stored.GetData(), v[0].data) || s.Stats().TailFetches != before {
		t.Fatalf("move did not borrow A: %v, %v", stored, err)
	}

	// C replaces the oldest user tail, which must still be A, not B.
	assertColdBlock(t, s, v[2*count])
	_, a := s.tails.resident.peek(0)
	_, b := s.tails.resident.peek(1)
	_, c := s.tails.resident.peek(2)
	if a || !b || !c {
		t.Fatalf("move reordered user eviction: resident A=%t B=%t C=%t, want false true true", a, b, c)
	}
}

// TestColdPackTailFallbacks proves both raw-reader paths use full wire keys.
func TestColdPackTailFallbacks(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		// Put a real tail one charge byte above the resident budget.
		v := coldValues(t, 1)[0]
		f := newColdFixture(t)
		f.addPack(t, []coldValue{v}, false)
		m := f.manifest(t)
		s := f.open(t, func(opts *ColdPackOptions) {
			opts.TailCacheBytes = int(coldTailCharge(m.Entries[0])) - 1
			opts.PromotedLog = ""
		})
		tail, err := s.tails.read(t.Context(), s.catalog.Load(), 0, false)
		if tail == nil || err != nil || tail.reader == nil || tail.entries != nil || s.tails.get(0) != nil {
			t.Fatalf("oversized tail not served through a transient raw reader: %v", err)
		}

		// A second body request reloads the raw tail and still verifies the block.
		assertColdBlock(t, s, v)
		if s.Stats().TailFetches != 2 || s.tails.get(0) != nil {
			t.Fatalf("oversized tail did not stay uncached: %+v", s.Stats())
		}
	})

	t.Run("extended-key", func(t *testing.T) {
		// Store an unknown protobuf field on an interior SHA-256 key. An
		// all-ones bloom makes the canonical key reach the exact raw lookup too.
		v := coldValues(t, 3)
		canonical := v[1].ref
		v[1] = coldExtendedValue(t, v[1])
		f := newColdFixture(t)
		f.addPack(t, v, true)
		f.manifest(t)
		s := f.open(t, func(opts *ColdPackOptions) { opts.PromotedLog = "" })

		// The extended key must find and verify its body through a raw tail.
		assertColdBlock(t, s, v[1])
		tail := s.tails.get(0)
		if tail == nil || tail.reader == nil || tail.entries != nil || s.Stats().TailFetches != 1 {
			t.Fatalf("extended index did not retain its raw wire keys: %+v", s.Stats())
		}

		// A negative for the canonical wire key must not hide the extended one.
		if yes, err := s.GetBlockExists(t.Context(), canonical); yes || err != nil {
			t.Fatalf("exact lookup conflated canonical and extended keys: %v, %v", yes, err)
		}
		assertColdBlock(t, s, v[1])
	})
}

// coldExtendedValue adds a wire field without changing the block's body digest.
func coldExtendedValue(t *testing.T, v coldValue) coldValue {
	// Retain the unknown field in the packed key instead of changing the payload.
	t.Helper()
	h := &hash.Hash{}
	if err := h.UnmarshalVT(append(packfile.BlockKey(v.ref.GetHash()), 0x18, 1)); err != nil {
		t.Fatal(err)
	}
	if n := h.SizeVT(); n <= coldKeySize {
		t.Fatalf("extended key fits scratch: %d bytes", n)
	}
	v.ref = block.NewBlockRef(h)
	return v
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
			f.appendTear(t, key, manifest.Entries[0], v[1], kind, s.promotions.recordCRC)

			// Keep warming out of this branch to measure an exact promoted GET.
			s = f.open(t, func(opts *ColdPackOptions) { opts.TailCacheBytes = 1 })
			checkColdLog(t, f.opts.PromotedLog, 1)
			before := f.reader.calls.Load()
			if yes, err := s.GetBlockExists(t.Context(), v[0].ref); yes || err != nil {
				t.Fatalf("unverified restored key said present: %v, %v", yes, err)
			}
			collisionHash := v[0].ref.GetHash().Clone()
			collisionHash.Hash[0] ^= 1
			collision := block.NewBlockRef(collisionHash)
			if yes, err := s.GetBlockExists(t.Context(), collision); yes || err != nil || f.reader.calls.Load() != before {
				t.Fatalf("different key with the old suffix answered %v, %v with %d reads", yes, err, f.reader.calls.Load()-before)
			}
			assertColdBlock(t, s, v[0])
			if s.Stats().TailFetches != 0 || f.reader.calls.Load() != before+1 {
				t.Fatalf("restored extent did not cost one exact read: %+v", s.Stats())
			}
			if yes, _ := s.GetBlockExists(t.Context(), collision); yes {
				t.Fatal("verified promotion authorized a different key with the old suffix")
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
	t.Run("stat-first", func(t *testing.T) {
		// A restored row has an extent, not evidence of a decoded size.
		for _, outcome := range []string{"verified", "range-failed"} {
			t.Run(outcome, func(t *testing.T) {
				// Persist a real record, then discard this mount's verification.
				v := coldValues(t, 1)[0]
				f := newColdFixture(t)
				key := f.addPack(t, []coldValue{v}, false)
				f.manifest(t)
				s := f.open(t, nil)
				assertColdBlock(t, s, v)
				waitCold(t, func() bool { return s.Stats().PromotedKeys == 1 })
				s.Close()

				// Make the body range fail authoritatively, so no fallback can hide it.
				if outcome == "range-failed" {
					f.setObject(key, "")
				}
				s = f.open(t, func(opts *ColdPackOptions) { opts.TailCacheBytes = 1 })
				if s.Stats().PromotedKeys != 1 || s.tails.get(0) != nil {
					t.Fatal("fixture must recover one record with no resident tail")
				}

				// Stat is the first read. One range and no tail fetch means it read the extent.
				before := f.reader.calls.Load()
				stat, err := s.StatBlock(t.Context(), v.ref)
				ranges := f.reader.calls.Load() - before
				if err != nil || ranges != 1 || s.Stats().TailFetches != 0 {
					t.Errorf("first stat must read the recovered extent: ranges=%d, tails=%d, err=%v", ranges, s.Stats().TailFetches, err)
				}
				if outcome == "range-failed" {
					if stat != nil {
						t.Errorf("first stat after failed range = %v; want no stat", stat)
					}
				} else if want := int64(len(v.data)); stat == nil || stat.Size != want {
					t.Errorf("first stat = %v; want verified size %d", stat, want)
				}
			})
		}
	})
	t.Run("legacy-extended-key", func(t *testing.T) {
		// Put E in an authenticated index whose canonical interior key C is absent.
		v := coldValues(t, 3)
		canonical := v[1].ref
		v[1] = coldExtendedValue(t, v[1])
		f := newColdFixture(t)
		key := f.addPack(t, v, true)
		f.manifest(t)
		f.seedLegacyPromotion(t, key, v[1])

		// A payload hash proves the data, not the distinct canonical wire key.
		s := f.open(t, func(opts *ColdPackOptions) { opts.TailCacheBytes = 1 })
		assertColdBlock(t, s, v[1])
		if stored, err := s.GetStoredBlock(t.Context(), canonical); stored != nil || err != nil {
			t.Errorf("legacy promotion returned the extended block for the canonical key: %v, %v", stored, err)
		}
		if yes, err := s.GetBlockExists(t.Context(), canonical); yes || err != nil {
			t.Errorf("legacy promotion authorized a canonical presence: %t, %v", yes, err)
		}
		if got := s.Stats().PromotedKeys; got != 0 {
			t.Errorf("retained %d records from an unqualified format", got)
		}
	})

	t.Run("manifest-change", func(t *testing.T) {
		// Persist a canonical record from a manifest with one pack.
		v := coldValues(t, 2)
		f := newColdFixture(t)
		f.addPack(t, v[:1], false)
		f.manifest(t)
		s := f.open(t, nil)
		assertColdBlock(t, s, v[0])
		waitCold(t, func() bool { return s.Stats().PromotedKeys == 1 })
		s.Close()

		// Replace the manifest without changing the old key's pack or ordinal.
		key := f.addPack(t, v[1:], false)
		f.setObject(key, f.objects[key])
		f.manifest(t)
		s = f.open(t, nil)
		if got := s.Stats().PromotedKeys; got != 0 {
			t.Errorf("recovered %d records from a different manifest", got)
		}
		assertColdBlock(t, s, v[0])
		waitCold(t, func() bool { return s.Stats().PromotedKeys == 1 })
	})
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

	// Hold the promoter's publication lock as well as its full queue.
	// Readers still return a verified block promptly.
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
	// ref holds the SHA-256 of data.
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
		return v.ref.GetHash(), &block.StoredBlock{Data: v.data, RefsSource: block.RefsAsserted}, nil
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
		filter, err := bloom.From([]uint64{^uint64(0)}, 64, 7)
		if err != nil {
			t.Fatal(err)
		}
		entry.BloomFilter, err = bloom.NewBloom(filter).MarshalVT()
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
	// Reuse the same bucket on restart. Copy options before overriding
	// them.
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

// seedLegacyPromotion writes the retired 32-byte format independently of the writer.
func (f *coldFixture) seedLegacyPromotion(t *testing.T, key string, v coldValue) {
	// Read the actual extended entry so both its extent and CRC are valid.
	t.Helper()
	body := []byte(f.objects[key])
	r, err := kvfile.BuildReader(bytes.NewReader(body), uint64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	e, _, err := r.SearchIndexEntryWithKey(packfile.BlockKey(v.ref.GetHash()))
	if err != nil || e == nil {
		t.Fatalf("find extended key in fixture: %v, %v", e, err)
	}

	// The old headerless row contains a suffix and the ordinal-zero extent.
	var raw [32]byte
	copy(raw[:12], v.ref.GetHash().GetHash()[20:])
	binary.BigEndian.PutUint16(raw[14:16], uint16(e.Offset>>32)) //nolint:gosec // The fixture pack fits 48 bits.
	binary.BigEndian.PutUint32(raw[16:20], uint32(e.Offset))     //nolint:gosec // The fixture pack fits 32 bits.
	binary.BigEndian.PutUint32(raw[20:24], uint32(e.Size))       //nolint:gosec // The fixture pack fits 32 bits.
	binary.BigEndian.PutUint32(raw[24:28], uint32(len(v.data)))  //nolint:gosec // The fixture block is short.
	binary.BigEndian.PutUint32(raw[28:], crc32.ChecksumIEEE(raw[:28]))
	if err := os.WriteFile(f.opts.PromotedLog, raw[:], 0o600); err != nil {
		t.Fatal(err)
	}
}

// appendTear writes an incomplete or bad-CRC record for v after a durable prefix.
func (f *coldFixture) appendTear(t *testing.T, key string, meta *packfile.ColdPackEntry, v coldValue, kind string, seed uint32) {
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
	record := encodeColdPromotion(nil, coldPromotion{extent: coldExtent{off: e.Offset, size: e.Size}, digest: digest}, seed)
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
	// client reads the range over HTTP.
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
	if err != nil || !bytes.Equal(stored.GetData(), v.data) || !stored.GetRefsSource().Recorded() || len(stored.GetRefs()) != 0 {
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
	if want := int64(coldPromotionHeaderBytes) + records*coldPromotionBytes; info.Size() != want {
		t.Fatalf("promoted log size %d; wanted %d", info.Size(), want)
	}
}

// _ is a type assertion
var _ ColdPackRangeReader = (*coldTestReader)(nil)
