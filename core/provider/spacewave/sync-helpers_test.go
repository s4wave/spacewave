package provider_spacewave

import (
	"bytes"
	"cmp"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/broadcast"
	packfile_manifest "github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

const (
	syncExecuteRequestTimeout = 2 * time.Second
	syncExecuteNoRetryWindow  = 1200 * time.Millisecond
	syncExecuteStopTimeout    = 500 * time.Millisecond
)

// syncPackTransport serves packed fixture bytes.
type syncPackTransport struct {
	data []byte
}

// syncErrorTransport returns a configured read failure.
type syncErrorTransport struct {
	err error
}

// syncCountingBlockStore observes upper-store block reads.
type syncCountingBlockStore struct {
	block.StoreOps
	onGet func(*block.BlockRef)
}

// Fetch reads a bounded range of fixture bytes.
func (t *syncPackTransport) Fetch(_ context.Context, off int64, length int) ([]byte, error) {
	if off >= int64(len(t.data)) {
		return nil, io.EOF
	}
	end := min(off+int64(length), int64(len(t.data)))
	return bytes.Clone(t.data[off:end]), nil
}

// Fetch returns the configured failure.
func (t *syncErrorTransport) Fetch(context.Context, int64, int) ([]byte, error) {
	return nil, t.err
}

// GetStoredBlock observes the read before forwarding it.
func (s *syncCountingBlockStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if s.onGet != nil {
		s.onGet(ref)
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// syncTestBlockStore stands in for the GC-backed bucket handle, which knows
// every block's refs. Test blocks carry none, so each reads as a leaf.
type syncTestBlockStore struct {
	block.StoreOps
}

// GetStoredBlock returns the block's bytes with an empty, known ref list.
func (s syncTestBlockStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := block.GetBlockWithoutRefs(ctx, s.StoreOps, ref)
	if stored != nil {
		stored.RefsSource = block.RefsAsserted
	}
	return stored, err
}

// newSyncTestBlockStore constructs the provider block test store.
func newSyncTestBlockStore() block.StoreOps {
	return syncTestBlockStore{StoreOps: newProviderSpacewaveTestBlockStore(hash.RecommendedHashType)}
}

// newSyncTestLowerPackfileStore packs fixture blocks into a readable lower store.
func newSyncTestLowerPackfileStore(t *testing.T, blocks map[string][]byte) *packfile_store.PackfileStore {
	// Hash each fixture block.
	t.Helper()
	var items []struct {
		h    *hash.Hash
		data []byte
	}
	for _, data := range blocks {
		h, err := hash.Sum(hash.RecommendedHashType, data)
		if err != nil {
			t.Fatalf("sum lower block hash: %v", err)
		}
		items = append(items, struct {
			h    *hash.Hash
			data []byte
		}{h: h, data: data})
	}

	// Pack the blocks in order.
	var buf bytes.Buffer
	idx := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(items) {
			return nil, nil, nil
		}
		item := items[idx]
		idx++
		return item.h, &block.StoredBlock{Data: item.data, RefsSource: block.RefsAsserted}, nil
	})
	if err != nil {
		t.Fatalf("pack lower blocks: %v", err)
	}

	// Serve the pack from memory.
	packData := bytes.Clone(buf.Bytes())
	lower := packfile_store.NewPackfileStore(
		func(packID string, size int64) (*packfile_store.PackReader, error) {
			return packfile_store.NewPackReader(
				packID,
				size,
				&syncPackTransport{data: packData},
			), nil
		},
		nil,
		nil,
	)

	// Make the failing pack a candidate for every requested block.
	lower.UpdateManifest([]*packfile.PackfileEntry{{
		Id:                 "existing-pack",
		BloomFilter:        result.BloomFilter,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         result.BlockCount,
		SizeBytes:          result.BytesWritten,
	}})
	return lower
}

// newSyncTestErrorLowerPackfileStore constructs a lower store whose reads fail.
func newSyncTestErrorLowerPackfileStore() *packfile_store.PackfileStore {
	// Fail every transport read from the lower store.
	lower := packfile_store.NewPackfileStore(
		func(packID string, size int64) (*packfile_store.PackReader, error) {
			return packfile_store.NewPackReader(
				packID,
				size,
				&syncErrorTransport{err: io.ErrUnexpectedEOF},
			), nil
		},
		nil, nil)
	lower.UpdateManifest([]*packfile.PackfileEntry{{
		Id:                 "error-pack",
		BloomFilter:        []byte("invalid-bloom"),
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         1,
		SizeBytes:          1,
	}})
	return lower
}

// syncTestLists is a fake of the child lists a Volume keeps, keyed by the
// block's string form.
type syncTestLists map[string][]*block.BlockRef

// GetChildren returns the children listed for ref.
func (l syncTestLists) GetChildren(_ context.Context, ref *block.BlockRef) ([]*block.BlockRef, bool, error) {
	children, ok := l[ref.MarshalString()]
	return slices.Clone(children), ok, nil
}

// newSyncTestKvStore constructs a transactional in-memory store.
func newSyncTestKvStore() kvtx.Store {
	return hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
}

// assertSyncPackEntryMetadata checks uploaded pack discovery metadata.
func assertSyncPackEntryMetadata(t *testing.T, entries []*packfile.PackfileEntry) {
	t.Helper()
	for i, entry := range entries {
		if entry.GetBlockCount() == 0 {
			t.Fatalf("entry %d missing block count", i)
		}
		if len(entry.GetBloomFilter()) == 0 {
			t.Fatalf("entry %d missing bloom filter", i)
		}
		if entry.GetCreatedAt() == nil {
			t.Fatalf("entry %d missing created-at metadata", i)
		}
	}
}

// readPackPhysicalKeys returns block keys in physical pack order.
func readPackPhysicalKeys(t *testing.T, body []byte) []string {
	// Read the pack index.
	t.Helper()
	reader, err := kvfile.BuildReader(bytes.NewReader(body), uint64(len(body)))
	if err != nil {
		t.Fatalf("build pack reader: %v", err)
	}

	// Collect the index entries in offset order.
	var entries []*kvfile.IndexEntry
	err = reader.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		entries = append(entries, ie.CloneVT())
		return nil
	})
	if err != nil {
		t.Fatalf("scan pack entries: %v", err)
	}
	slices.SortFunc(entries, func(a, b *kvfile.IndexEntry) int {
		return cmp.Compare(a.GetOffset(), b.GetOffset())
	})

	// Decode each entry's block key.
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		h, err := packfile.ParseBlockKey(entry.GetKey())
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, h.MarshalString())
	}
	return keys
}

// addSyncDirtyBlock stores a block and queues it for upload.
func addSyncDirtyBlock(
	t *testing.T,
	ctx context.Context,
	wtx kvtx.Tx,
	upper block.StoreOps,
	body string,
) *block.BlockRef {
	// Store the block and mark it for upload.
	t.Helper()
	data := []byte(body)
	ref, _, err := upper.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatalf("put upper block: %v", err)
	}
	if _, err := markPendingUploads(ctx, wtx, []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len(data))}}); err != nil {
		t.Fatalf("set dirty key: %v", err)
	}
	return ref
}

// newDirtySyncExecuteTestController constructs a controller with pending upload work.
func newDirtySyncExecuteTestController(
	t *testing.T,
	cli *SessionClient,
	gate *broadcast.Broadcast,
) *syncController {
	// Build the dirty and manifest stores.
	t.Helper()
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	manifestStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, manifestStore)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Queue one dirty block.
	upper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	addSyncDirtyBlock(t, ctx, wtx, upper, "dirty block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// Build the controller over the stores.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     func() (*SessionClient, error) { return cli, nil },
		resourceID: "test-res",
		mfst:       mfst,
		lower:      packfile_store.NewPackfileStore(nil, nil, nil),
		upper:      upper,
		conf:       &SyncConfig{SizeThresholdBytes: 1, CheckpointIntervalSecs: 1},
		gateBcast:  gate,
	}
	s.updateDirtyState(ctx)
	return s
}

// syncTestChunkBlockBytes sizes test blocks so two fill one sync pack and a
// third starts the next.
const syncTestChunkBlockBytes = int(syncFlushMaxPackBytes/2) - 64*1024

// countSyncDirtyKeys returns the number of dirty markers in store.
func countSyncDirtyKeys(t *testing.T, ctx context.Context, store kvtx.Store) int {
	// Scan the dirty prefix in a read transaction.
	t.Helper()
	rtx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("new read tx: %v", err)
	}
	defer rtx.Discard()
	var count int
	if err := rtx.ScanPrefix(ctx, []byte("dirty/"), func(_, _ []byte) error {
		count++
		return nil
	}); err != nil {
		t.Fatalf("scan dirty keys: %v", err)
	}
	return count
}

// addSyncChunkBlocks stores n distinct blocks of syncTestChunkBlockBytes in
// upper and marks them dirty in store.
func addSyncChunkBlocks(t *testing.T, ctx context.Context, store kvtx.Store, upper block.StoreOps, n int) {
	// Open the dirty marker transaction.
	t.Helper()
	wtx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()

	// Store and mark each block.
	for i := range n {
		data := bytes.Repeat([]byte{byte(i + 1)}, syncTestChunkBlockBytes)
		ref, _, err := upper.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatalf("put upper block: %v", err)
		}
		if _, err := markPendingUploads(ctx, wtx, []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len(data))}}); err != nil {
			t.Fatalf("set dirty key: %v", err)
		}
	}

	// Commit the markers.
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}
}

// corruptUpperStore reads one block as corrupt, as an upper store whose disk
// returned the wrong bytes does, and serves the rest.
type corruptUpperStore struct {
	block.StoreOps
	bad *hash.Hash
}

func (s *corruptUpperStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if ref.GetHash().EqualVT(s.bad) {
		return nil, &block.CorruptError{Ref: ref}
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// pausingReadStore pauses its pauseAt-th read transaction until release
// closes, so a test can act between two reads of one flush.
type pausingReadStore struct {
	kvtx.Store
	reads   atomic.Int64
	pauseAt atomic.Int64
	paused  chan struct{}
	release chan struct{}
}

// NewTransaction pauses the armed read transaction.
func (s *pausingReadStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		if n := s.reads.Add(1); n == s.pauseAt.Load() {
			close(s.paused)
			<-s.release
		}
	}
	return s.Store.NewTransaction(ctx, write)
}

// heldTestController builds a sync controller over a server that fails every
// push from the failFrom-th on while failFrom is not zero, with telemetry, so
// a test reads the held state the sync status shows. Its queue holds the
// "dirty block" first, then five chunk-sized blocks that fill three chunks.
// The upper store reads the hashes in bad as corrupt.
func heldTestController(t *testing.T) (*syncController, *atomic.Int32, *atomic.Int32, *corruptSetStore) {
	// Fail pushes from failFrom on and count them.
	var failFrom, pushes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/push") {
			if n, f := pushes.Add(1), failFrom.Load(); f != 0 && n >= f {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, server.URL, DefaultSigningEnvPrefix, priv, pid.String())

	// Queue the chunk-sized blocks behind the dirty block and read through
	// the corrupt set.
	s := newDirtySyncExecuteTestController(t, cli, nil)
	s.telemetry = &NewTestProviderAccount(t, server.URL).syncTelemetry
	addSyncChunkBlocks(t, t.Context(), s.store, s.upper, 5)
	s.updateDirtyState(t.Context())
	upper := &corruptSetStore{StoreOps: s.upper, bad: map[string]struct{}{}}
	s.upper = upper
	return s, &failFrom, &pushes, upper
}

// corruptSetStore reads each block whose hash is in bad as corrupt.
type corruptSetStore struct {
	block.StoreOps
	bad map[string]struct{}
}

func (s *corruptSetStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if hasKey(s.bad, ref.GetHash().MarshalString()) {
		return nil, &block.CorruptError{Ref: ref}
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// heldStatus returns the held state the sync status shows for the test store.
func heldStatus(t *testing.T, s *syncController) (int, bool, time.Time) {
	t.Helper()
	for _, b := range s.telemetry.Snapshot().BlockStores {
		if b.BlockStoreID == s.resourceID {
			return b.CorruptDirtyCount, b.HeldPartial, b.HeldSince
		}
	}
	return 0, false, time.Time{}
}

// wantHold checks the controller's held set and the status it shows.
func wantHold(t *testing.T, stage string, s *syncController, keys []string, partial bool) {
	t.Helper()
	var hold holdState
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { hold = s.hold })
	count, shownPartial, since := heldStatus(t, s)
	held := len(keys) != 0
	if len(hold.blocks) != len(keys) || hold.partial != partial || hold.since.IsZero() == held ||
		count != len(keys) || shownPartial != partial || !since.Equal(hold.since) {
		t.Fatalf("%s: held %d partial %v since %v, shown %d partial %v since %v; want %d held, partial %v",
			stage, len(hold.blocks), hold.partial, hold.since, count, shownPartial, since, len(keys), partial)
	}
	for _, key := range keys {
		if !hasKey(hold.blocks, key) {
			t.Fatalf("%s: a block that must stay held is not", stage)
		}
	}
}

// wantMarker checks that the dirty marker and the stored bytes of a damaged
// block are still there.
func wantMarker(t *testing.T, stage string, s *syncController, upper *corruptSetStore, ref *block.BlockRef) {
	t.Helper()
	var marked bool
	err := kvtx.RunTransaction(t.Context(), false,
		func(ctx context.Context) (kvtx.Tx, error) { return s.store.NewTransaction(ctx, false) },
		func(ctx context.Context, tx kvtx.Tx) error {
			var err error
			marked, err = tx.Exists(ctx, []byte("dirty/"+ref.GetHash().MarshalString()))
			return err
		},
	)
	if err != nil || !marked {
		t.Fatalf("%s: the damaged block's marker is gone: %v", stage, err)
	}
	if found, err := upper.GetBlockExists(t.Context(), ref); err != nil || !found {
		t.Fatalf("%s: the damaged block's bytes are gone: %v", stage, err)
	}
}
