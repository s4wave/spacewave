//go:build js

package blockshard

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strconv"
	"syscall/js"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

func newTestEngine(t testing.TB, dirName, lockPrefix string) (*Engine, func()) {
	return newTestEngineWithSettings(t, dirName, lockPrefix, nil)
}

func newTestEngineWithSettings(t testing.TB, dirName, lockPrefix string, settings *Settings) (*Engine, func()) {
	t.Helper()
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, dirName, true)
	if err != nil {
		t.Fatal(err)
	}
	if settings == nil {
		settings = DefaultSettings()
		settings.ShardCount = 1
	}
	e, err := NewEngineWithSettings(context.Background(), dir, lockPrefix, settings)
	if err != nil {
		t.Fatal(err)
	}
	return e, func() {
		e.Close()
		_ = opfs.DeleteEntry(root, dirName, true)
	}
}

func publishEntries(t testing.TB, s *Shard, entries []segment.Entry) {
	t.Helper()
	release, err := s.AcquirePublishLock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := s.Publish(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReclaimPendingDelete(); err != nil {
		t.Fatal(err)
	}
}

func compactShard(t testing.TB, s *Shard) {
	t.Helper()
	plan := PlanCompaction(s, DefaultL0Trigger)
	if plan == nil {
		t.Fatal("expected compaction plan")
	}
	release, err := s.AcquirePublishLock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := ExecuteCompaction(s, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReclaimPendingDelete(); err != nil {
		t.Fatal(err)
	}
}

func TestBroadcastChannelInvalidationUsesTypedArrayPayload(t *testing.T) {
	b := NewBroadcaster("test-typed-array")
	defer b.Close()
	l := NewListener("test-typed-array")
	defer l.Close()

	const shardID uint16 = 0x8003
	const generation uint64 = 0xfedcba9876543210
	b.Send(int(shardID), generation)

	select {
	case <-l.Notify():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for invalidation message")
	}

	msgs := l.DrainPending()
	if len(msgs) != 1 {
		t.Fatalf("pending messages: got %d, want 1", len(msgs))
	}
	if msgs[0].ShardID != shardID || msgs[0].Generation != generation {
		t.Fatalf("pending message = %+v, want shard=%d generation=%d", msgs[0], shardID, generation)
	}
}

func TestBroadcastChannelInvalidationAcceptsArrayBufferPayload(t *testing.T) {
	l := NewListener("test-array-buffer")
	defer l.Close()

	const shardID uint16 = 0x7004
	const generation uint64 = 0x0123456789abcdef
	arr := js.Global().Get("Uint8Array").New(10)
	arr.SetIndex(0, int(shardID>>8))
	arr.SetIndex(1, int(shardID))
	for i := 0; i < 8; i++ {
		shift := uint((7 - i) * 8)
		arr.SetIndex(2+i, int(byte(generation>>shift)))
	}
	event := js.Global().Get("Object").New()
	event.Set("data", arr.Get("buffer"))

	l.cleanup.Invoke(event)
	select {
	case <-l.Notify():
	default:
		t.Fatal("array buffer payload did not notify listener")
	}

	msgs := l.DrainPending()
	if len(msgs) != 1 {
		t.Fatalf("pending messages: got %d, want 1", len(msgs))
	}
	if msgs[0].ShardID != shardID || msgs[0].Generation != generation {
		t.Fatalf("pending message = %+v, want shard=%d generation=%d", msgs[0], shardID, generation)
	}
}

func TestBroadcastChannelInvalidationScopesByLockPrefix(t *testing.T) {
	b := NewBroadcaster("test-scope-a")
	defer b.Close()
	l := NewListener("test-scope-b")
	defer l.Close()

	b.Send(1, 2)
	select {
	case <-l.Notify():
		t.Fatal("listener received invalidation from another blockshard scope")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSingletonPutDoesNotWaitForFlushAge(t *testing.T) {
	settings := DefaultSettings()
	settings.ShardCount = 1

	e, cleanup := newTestEngineWithSettings(
		t,
		"test-blockshard-singleton-no-wait",
		"test-blockshard-singleton-no-wait",
		settings,
	)
	defer cleanup()

	start := time.Now()
	if err := e.Put(context.Background(), []segment.Entry{{
		Key:   []byte("singleton"),
		Value: []byte("value"),
	}}); err != nil {
		t.Fatal(err)
	}
	dur := time.Since(start)
	if dur >= 200*time.Millisecond {
		t.Fatalf("singleton put took %v; expected no pre-publish wait", dur)
	}

	val, found, err := e.Get([]byte("singleton"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "value" {
		t.Fatalf("singleton get: found=%v val=%q want value", found, val)
	}
}

func TestAsyncIOWriteAndRead(t *testing.T) {
	settings := DefaultSettings()
	settings.ShardCount = 1
	settings.AsyncIO = true

	e, cleanup := newTestEngineWithSettings(
		t,
		"test-blockshard-async-io",
		"test-blockshard-async-io",
		settings,
	)
	defer cleanup()

	if err := e.Put(context.Background(), []segment.Entry{{
		Key:   []byte("async"),
		Value: []byte("mode"),
	}}); err != nil {
		t.Fatal(err)
	}

	val, found, err := e.Get([]byte("async"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "mode" {
		t.Fatalf("async get: found=%v val=%q want mode", found, val)
	}
}

func TestAsyncIOConsecutivePutsDoNotWriteManifestGenerationPointer(t *testing.T) {
	settings := DefaultSettings()
	settings.ShardCount = 1
	settings.AsyncIO = true

	e, cleanup := newTestEngineWithSettings(
		t,
		"test-blockshard-async-io-consecutive-puts",
		"test-blockshard-async-io-consecutive-puts",
		settings,
	)
	defer cleanup()

	if err := e.Put(context.Background(), []segment.Entry{{
		Key:   []byte("a"),
		Value: []byte("first"),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := e.Put(context.Background(), []segment.Entry{{
		Key:   []byte("b"),
		Value: []byte("second"),
	}}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		key  string
		want string
	}{
		{key: "a", want: "first"},
		{key: "b", want: "second"},
	} {
		val, found, err := e.Get([]byte(tc.key))
		if err != nil {
			t.Fatal(err)
		}
		if !found || string(val) != tc.want {
			t.Fatalf("get %q: found=%v val=%q want %q", tc.key, found, val, tc.want)
		}
	}

	_, err := opfs.ReadFile(e.shards[0].dir, "manifest-gen")
	if err == nil || !opfs.IsNotFound(err) {
		t.Fatalf("manifest-gen read error = %v, want not found", err)
	}
}

func TestWriteFileDataAsyncReplacesWholeFile(t *testing.T) {
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dirName := "test-blockshard-async-replace-file"
	dir, err := opfs.GetDirectory(root, dirName, true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, dirName, true) //nolint

	settings := DefaultSettings()
	settings.ShardCount = 1
	settings.AsyncIO = true
	shard, err := NewShard(0, dir, dirName, settings)
	if err != nil {
		t.Fatal(err)
	}

	const name = "seg-000001.sst"
	if err := shard.writeFileData(context.Background(), name, []byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	if err := shard.writeFileData(context.Background(), name, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	got, err := opfs.ReadFile(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abc" {
		t.Fatalf("file content after async replace = %q, want abc", string(got))
	}
}

type fakeSegmentReader struct {
	buf   []byte
	reads int
}

func (f *fakeSegmentReader) ReadAt(p []byte, off int64) (int, error) {
	f.reads++
	if off >= int64(len(f.buf)) {
		return 0, io.EOF
	}
	n := copy(p, f.buf[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *fakeSegmentReader) Size() (int64, error) {
	return int64(len(f.buf)), nil
}

func TestCachedSegmentFileCachesWindow(t *testing.T) {
	data := bytes.Repeat([]byte("abcdefghijklmnopqrstuvwxyz"), (cachedSegmentBlockSize*3)/26+2)
	rd := &fakeSegmentReader{
		buf: data[:cachedSegmentBlockSize*3],
	}
	f := newCachedSegmentFile(rd, int64(len(rd.buf)))

	buf := make([]byte, 16)
	off := int64(cachedSegmentBlockSize - 8)
	want := string(rd.buf[off : off+16])
	if _, err := f.ReadAt(buf, off); err != nil {
		t.Fatal(err)
	}
	if got := string(buf); got != want {
		t.Fatalf("first read got %q want %q", got, want)
	}
	if rd.reads != 2 {
		t.Fatalf("reads after cross-block call: got %d want 2", rd.reads)
	}

	buf = make([]byte, 8)
	off = int64(cachedSegmentBlockSize - 4)
	want = string(rd.buf[off : off+8])
	if _, err := f.ReadAt(buf, off); err != nil {
		t.Fatal(err)
	}
	if got := string(buf); got != want {
		t.Fatalf("cached overlap got %q want %q", got, want)
	}
	if rd.reads != 2 {
		t.Fatalf("cached overlap should not reread: got %d reads", rd.reads)
	}

	buf = make([]byte, 4)
	if _, err := f.ReadAt(buf, int64(cachedSegmentBlockSize*2)); err != nil {
		t.Fatal(err)
	}
	if rd.reads != 3 {
		t.Fatalf("third block should reread once: got %d reads", rd.reads)
	}
}

func TestShardCachesSegmentFiles(t *testing.T) {
	e, cleanup := newTestEngine(t, "test-blockshard-segment-cache", "test-blockshard-segment-cache")
	defer cleanup()

	if err := e.Put(context.Background(), []segment.Entry{{
		Key:   []byte("cached"),
		Value: []byte("value"),
	}}); err != nil {
		t.Fatal(err)
	}

	m := e.shards[0].Manifest()
	if len(m.Segments) != 1 {
		t.Fatalf("segments: got %d want 1", len(m.Segments))
	}
	seg := &m.Segments[0]

	f1, err := e.shards[0].getSegmentFile(context.Background(), seg)
	if err != nil {
		t.Fatal(err)
	}
	f2, err := e.shards[0].getSegmentFile(context.Background(), seg)
	if err != nil {
		t.Fatal(err)
	}
	if f1 != f2 {
		t.Fatal("expected cached segment file handle")
	}

	e.shards[0].mu.Lock()
	e.shards[0].setManifestLocked(&Manifest{Generation: m.Generation + 1})
	_, ok := e.shards[0].segmentFileCache[seg.Filename]
	e.shards[0].mu.Unlock()
	if ok {
		t.Fatal("expected segment file cache eviction after manifest update")
	}
}

func TestBlockStoreGetBlockExists(t *testing.T) {
	e, cleanup := newTestEngine(t, "test-blockshard-store-exists", "test-blockshard-store-exists")
	defer cleanup()

	store := NewBlockStore(e, block.DefaultHashType)
	ref, existed, err := store.PutBlock(context.Background(), []byte("value"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if existed {
		t.Fatal("put should create a new block")
	}

	found, err := store.GetBlockExists(context.Background(), ref.Clone())
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("GetBlockExists(existing): not found")
	}

	if err := store.RmBlock(context.Background(), ref.Clone()); err != nil {
		t.Fatal(err)
	}

	found, err = store.GetBlockExists(context.Background(), ref.Clone())
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("GetBlockExists(tombstoned): should not be found")
	}

	_, found, err = store.GetBlock(context.Background(), ref.Clone())
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("GetBlock(tombstoned): should not be found")
	}
}

func TestBlockStoreGetBlockExistsBatch(t *testing.T) {
	e, cleanup := newTestEngine(t, "test-blockshard-store-exists-batch", "test-blockshard-store-exists-batch")
	defer cleanup()

	store := NewBlockStore(e, block.DefaultHashType)
	ref1, _, err := store.PutBlock(context.Background(), []byte("value-1"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ref2, _, err := store.PutBlock(context.Background(), []byte("value-2"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ref3, err := block.BuildBlockRef([]byte("missing"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RmBlock(context.Background(), ref2.Clone()); err != nil {
		t.Fatal(err)
	}

	found, err := store.GetBlockExistsBatch(context.Background(), []*block.BlockRef{
		ref1.Clone(),
		ref2.Clone(),
		ref3.Clone(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 3 {
		t.Fatalf("expected 3 batch results, got %d", len(found))
	}
	if !found[0] {
		t.Fatal("GetBlockExistsBatch(existing): not found")
	}
	if found[1] {
		t.Fatal("GetBlockExistsBatch(tombstoned): should not be found")
	}
	if found[2] {
		t.Fatal("GetBlockExistsBatch(missing): should not be found")
	}
}

func TestWritePolicy(t *testing.T) {
	tests := []struct {
		name      string
		asyncIO   bool
		filename  string
		wantAsync bool
	}{
		{
			name:      "force-async-segment",
			asyncIO:   true,
			filename:  "seg-000001.sst",
			wantAsync: true,
		},
		{
			name:      "force-async-manifest",
			asyncIO:   true,
			filename:  manifestSlotA,
			wantAsync: true,
		},
		{
			name:      "default-manifest",
			asyncIO:   false,
			filename:  manifestSlotA,
			wantAsync: true,
		},
		{
			name:      "default-segment",
			asyncIO:   false,
			filename:  "seg-000001.sst",
			wantAsync: !opfs.SyncAvailable(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Shard{asyncIO: tt.asyncIO}
			if got := s.shouldUseAsyncWrite(tt.filename); got != tt.wantAsync {
				t.Fatalf("shouldUseAsyncWrite(%q) = %v, want %v", tt.filename, got, tt.wantAsync)
			}
		})
	}
}

func TestAsyncIOPutComparison(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	ctx := context.Background()
	syncSettings := DefaultSettings()
	syncSettings.ShardCount = 4

	asyncSettings := DefaultSettings()
	asyncSettings.ShardCount = 4
	asyncSettings.AsyncIO = true

	syncSingle := measureSinglePutLatency(t, ctx, "test-blockshard-compare-sync-single", syncSettings, 24)
	asyncSingle := measureSinglePutLatency(t, ctx, "test-blockshard-compare-async-single", asyncSettings, 24)
	t.Logf(
		"single put sync avg=%s p50=%s p95=%s max=%s | async avg=%s p50=%s p95=%s max=%s",
		syncSingle.avg,
		syncSingle.p50,
		syncSingle.p95,
		syncSingle.max,
		asyncSingle.avg,
		asyncSingle.p50,
		asyncSingle.p95,
		asyncSingle.max,
	)

	syncBatch := measureBatchPutLatency(t, ctx, "test-blockshard-compare-sync-batch", syncSettings, 12, 32)
	asyncBatch := measureBatchPutLatency(t, ctx, "test-blockshard-compare-async-batch", asyncSettings, 12, 32)
	t.Logf(
		"batch put sync avg=%s p50=%s p95=%s max=%s | async avg=%s p50=%s p95=%s max=%s",
		syncBatch.avg,
		syncBatch.p50,
		syncBatch.p95,
		syncBatch.max,
		asyncBatch.avg,
		asyncBatch.p50,
		asyncBatch.p95,
		asyncBatch.max,
	)
}

func TestStaleReaderRefreshesAfterCompactionReclaim(t *testing.T) {
	writer, cleanupWriter := newTestEngine(t, "test-blockshard-stale-reader", "test-blockshard-stale-reader")
	defer cleanupWriter()
	reader, cleanupReader := newTestEngine(t, "test-blockshard-stale-reader", "test-blockshard-stale-reader")
	defer cleanupReader()

	now := time.UnixMilli(1000)
	writer.shards[0].nowFn = func() time.Time { return now }

	key := []byte("block-key")
	for _, v := range []string{"v1", "v2", "v3", "v4"} {
		publishEntries(t, writer.shards[0], []segment.Entry{{
			Key:   key,
			Value: []byte(v),
		}})
	}
	if _, err := reader.refreshShardManifest(0); err != nil {
		t.Fatal(err)
	}
	stale := reader.shards[0].Manifest()
	if len(stale.Segments) != 4 {
		t.Fatalf("stale manifest segments: got %d want 4", len(stale.Segments))
	}

	compactShard(t, writer.shards[0])
	postCompact := writer.shards[0].Manifest()
	if len(postCompact.PendingDelete) != 4 {
		t.Fatalf("post-compaction pending deletes: got %d want 4", len(postCompact.PendingDelete))
	}

	missing := stale.Segments[len(stale.Segments)-1].Filename
	foundPending := false
	for _, seg := range postCompact.PendingDelete {
		if seg.Filename == missing {
			foundPending = true
			break
		}
	}
	if !foundPending {
		t.Fatalf("expected %q in pending delete set after compaction", missing)
	}

	now = now.Add(DefaultRetireGracePeriod + time.Millisecond)
	publishEntries(t, writer.shards[0], []segment.Entry{{
		Key:   key,
		Value: []byte("v5"),
	}})
	beforeReclaim := writer.shards[0].Manifest()
	if len(beforeReclaim.PendingDelete) != 4 {
		t.Fatalf("pending deletes before generation gate: got %d want 4", len(beforeReclaim.PendingDelete))
	}
	exists, err := opfs.FileExists(writer.shards[0].dir, missing)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("pending segment %q was removed before generation gate", missing)
	}

	publishEntries(t, writer.shards[0], []segment.Entry{{
		Key:   key,
		Value: []byte("v6"),
	}})
	current := writer.shards[0].Manifest()
	if len(current.PendingDelete) != 0 {
		t.Fatalf("expected reclaimed pending deletes, got %d at generation %d", len(current.PendingDelete), current.Generation)
	}

	reader.shards[0].mu.Lock()
	reader.shards[0].manifest = stale.Clone()
	reader.shards[0].mu.Unlock()
	reader.shards[0].observeGeneration(current.Generation)

	val, found, err := reader.GetFromShard(0, key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v6" {
		t.Fatalf("stale reader result: found=%v val=%q want v6", found, val)
	}
}

func TestSerializedPublishReloadsStaleManifest(t *testing.T) {
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-blockshard-stale-publish", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-blockshard-stale-publish", true) //nolint

	settings := DefaultSettings()
	settings.ShardCount = 1
	first, err := NewShard(0, dir, "test-blockshard-stale-publish", settings)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewShard(0, dir, "test-blockshard-stale-publish", settings)
	if err != nil {
		t.Fatal(err)
	}

	if gen := second.Manifest().Generation; gen != 0 {
		t.Fatalf("second shard initial generation: got %d want 0", gen)
	}

	keyA := []byte("a")
	keyB := []byte("b")
	publishEntries(t, first, []segment.Entry{{Key: keyA, Value: []byte("one")}})
	publishEntries(t, second, []segment.Entry{{Key: keyB, Value: []byte("two")}})

	fresh, err := NewShard(0, dir, "test-blockshard-stale-publish", settings)
	if err != nil {
		t.Fatal(err)
	}
	m := fresh.Manifest()
	if len(m.Segments) != 2 {
		t.Fatalf("segments after two serialized publishes: got %d want 2", len(m.Segments))
	}
	assertShardEntry(t, fresh, keyA, []byte("one"))
	assertShardEntry(t, fresh, keyB, []byte("two"))
}

func TestPublishPreservesPendingDelete(t *testing.T) {
	e, cleanup := newTestEngine(t, "test-blockshard-publish-pending-delete", "test-blockshard-publish-pending-delete")
	defer cleanup()

	for _, key := range []string{"a", "b", "c", "d"} {
		publishEntries(t, e.shards[0], []segment.Entry{{
			Key:   []byte(key),
			Value: []byte("value-" + key),
		}})
	}
	compactShard(t, e.shards[0])
	compacted := e.shards[0].Manifest()
	if len(compacted.PendingDelete) != 4 {
		t.Fatalf("pending delete after compaction: got %d want 4", len(compacted.PendingDelete))
	}

	publishEntries(t, e.shards[0], []segment.Entry{{
		Key:   []byte("e"),
		Value: []byte("value-e"),
	}})
	current := e.shards[0].Manifest()
	if len(current.PendingDelete) != len(compacted.PendingDelete) {
		t.Fatalf("pending delete after publish: got %d want %d", len(current.PendingDelete), len(compacted.PendingDelete))
	}
	assertShardEntry(t, e.shards[0], []byte("e"), []byte("value-e"))
}

func TestCompactionIncludesOverlappingLowerLevelAndAgesTombstone(t *testing.T) {
	e, cleanup := newTestEngine(t, "test-blockshard-overlap-tombstone-aging", "test-blockshard-overlap-tombstone-aging")
	defer cleanup()

	for _, key := range []string{"a", "b", "c", "x"} {
		publishEntries(t, e.shards[0], []segment.Entry{{
			Key:   []byte(key),
			Value: []byte("old-" + key),
		}})
	}
	compactShard(t, e.shards[0])

	for _, entry := range []segment.Entry{
		{Key: []byte("d"), Value: []byte("new-d")},
		{Key: []byte("e"), Value: []byte("new-e")},
		{Key: []byte("f"), Value: []byte("new-f")},
		{Key: []byte("x"), Tombstone: true},
	} {
		publishEntries(t, e.shards[0], []segment.Entry{entry})
	}
	compactShard(t, e.shards[0])

	if _, found, err := e.Get([]byte("x")); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("compacted tombstone did not suppress older lower-level value")
	}
	for _, key := range []string{"d", "e", "f"} {
		val, found, err := e.Get([]byte(key))
		if err != nil {
			t.Fatal(err)
		}
		if !found || string(val) != "new-"+key {
			t.Fatalf("key %q: found=%v val=%q", key, found, val)
		}
	}

	stats, err := e.LevelStats()
	if err != nil {
		t.Fatal(err)
	}
	level1 := findLevelStats(stats, 1)
	if level1 == nil {
		t.Fatalf("missing level 1 stats: %+v", stats)
	}
	if level1.TombstoneCount != 0 {
		t.Fatalf("tombstone count after aging: got %d want 0", level1.TombstoneCount)
	}
	if level1.LiveEntryCount != 6 {
		t.Fatalf("live entry count: got %d want 6", level1.LiveEntryCount)
	}
	if level1.PendingDeleteSegmentCount == 0 {
		t.Fatalf("expected pending-delete stats after compaction: %+v", stats)
	}
}

func TestCleanOrphansRemovesInterruptedCompactionOutput(t *testing.T) {
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-blockshard-clean-compaction-orphan", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-blockshard-clean-compaction-orphan", true) //nolint

	settings := DefaultSettings()
	settings.ShardCount = 1
	shard, err := NewShard(0, dir, "test-blockshard-clean-compaction-orphan", settings)
	if err != nil {
		t.Fatal(err)
	}
	publishEntries(t, shard, []segment.Entry{{Key: []byte("live"), Value: []byte("value")}})
	if err := shard.writeFileData(context.Background(), "seg-000999.sst", buildSegmentData(t, []segment.Entry{{
		Key:   []byte("orphan"),
		Value: []byte("value"),
	}})); err != nil {
		t.Fatal(err)
	}
	if err := shard.CleanOrphans(); err != nil {
		t.Fatal(err)
	}
	exists, err := opfs.FileExists(dir, "seg-000999.sst")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("interrupted compaction output was not removed")
	}
	assertShardEntry(t, shard, []byte("live"), []byte("value"))
}

func TestCleanOrphansReloadsStaleManifest(t *testing.T) {
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-blockshard-clean-orphans-reload", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-blockshard-clean-orphans-reload", true) //nolint

	settings := DefaultSettings()
	settings.ShardCount = 1
	writer, err := NewShard(0, dir, "test-blockshard-clean-orphans-reload", settings)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := NewShard(0, dir, "test-blockshard-clean-orphans-reload", settings)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("committed")
	publishEntries(t, writer, []segment.Entry{{Key: key, Value: []byte("value")}})

	if err := stale.CleanOrphans(); err != nil {
		t.Fatal(err)
	}
	fresh, err := NewShard(0, dir, "test-blockshard-clean-orphans-reload", settings)
	if err != nil {
		t.Fatal(err)
	}
	assertShardEntry(t, fresh, key, []byte("value"))
}

func findLevelStats(stats []LevelStats, level uint8) *LevelStats {
	for i := range stats {
		if stats[i].Level == level {
			return &stats[i]
		}
	}
	return nil
}

func buildSegmentData(t testing.TB, entries []segment.Entry) []byte {
	t.Helper()
	w := segment.NewWriter()
	for _, entry := range entries {
		if entry.Tombstone {
			w.AddTombstone(entry.Key)
			continue
		}
		w.Add(entry.Key, entry.Value)
	}
	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func assertShardEntry(t testing.TB, shard *Shard, key, want []byte) {
	t.Helper()
	m := shard.Manifest()
	for i := len(m.Segments) - 1; i >= 0; i-- {
		seg := &m.Segments[i]
		if string(key) < string(seg.MinKey) || string(key) > string(seg.MaxKey) {
			continue
		}
		lookup, err := shard.getLookup(context.Background(), seg)
		if err != nil {
			t.Fatal(err)
		}
		f, err := shard.getSegmentFile(context.Background(), seg)
		if err != nil {
			t.Fatal(err)
		}
		val, found, tombstone, err := lookup.Locate(f, key, true)
		if err != nil {
			t.Fatal(err)
		}
		if tombstone {
			t.Fatalf("key %q resolved to tombstone", key)
		}
		if !found {
			continue
		}
		if !bytes.Equal(val, want) {
			t.Fatalf("key %q: got %q want %q", key, val, want)
		}
		return
	}
	t.Fatalf("key %q not reachable from manifest", key)
}

func BenchmarkBlockshardPutBatchMatrix(b *testing.B) {
	ctx := context.Background()
	compactionTriggers := []int{DefaultL0Trigger, DefaultL0Trigger * 2}

	for _, shardCount := range []int{4, 8} {
		for _, compactionTrigger := range compactionTriggers {
			name := "shards-" + strconv.Itoa(shardCount) + "/compact-" + strconv.Itoa(compactionTrigger)
			b.Run(name, func(b *testing.B) {
				settings := DefaultSettings()
				settings.AsyncIO = true
				settings.ShardCount = shardCount
				settings.CompactionTrigger = compactionTrigger

				engine, cleanup := newTestEngineWithSettings(
					b,
					"bench-blockshard-"+strconv.Itoa(shardCount)+"-"+strconv.Itoa(compactionTrigger),
					"bench-blockshard-"+strconv.Itoa(shardCount)+"-"+strconv.Itoa(compactionTrigger),
					settings,
				)
				b.Cleanup(cleanup)
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; b.Loop(); i++ {
					if err := engine.Put(ctx, buildBenchmarkEntries(i*32, 32)); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func buildBenchmarkEntries(start, count int) []segment.Entry {
	entries := make([]segment.Entry, count)
	for i := range count {
		n := start + i
		entries[i] = segment.Entry{
			Key:   []byte("bench-key-" + strconv.Itoa(n)),
			Value: []byte("bench-value-" + strconv.Itoa(n)),
		}
	}
	return entries
}

type latencyStats struct {
	avg time.Duration
	p50 time.Duration
	p95 time.Duration
	max time.Duration
}

func measureSinglePutLatency(
	t *testing.T,
	ctx context.Context,
	dirName string,
	settings *Settings,
	count int,
) latencyStats {
	t.Helper()
	e, cleanup := newTestEngineWithSettings(t, dirName, dirName, settings)
	defer cleanup()

	durs := make([]time.Duration, 0, count)
	for i := range count {
		start := time.Now()
		err := e.Put(ctx, []segment.Entry{{
			Key:   []byte("single-" + strconv.Itoa(i)),
			Value: make([]byte, 4096),
		}})
		if err != nil {
			t.Fatal(err)
		}
		durs = append(durs, time.Since(start))
	}
	return buildLatencyStats(durs)
}

func measureBatchPutLatency(
	t *testing.T,
	ctx context.Context,
	dirName string,
	settings *Settings,
	rounds int,
	batchSize int,
) latencyStats {
	t.Helper()
	e, cleanup := newTestEngineWithSettings(t, dirName, dirName, settings)
	defer cleanup()

	durs := make([]time.Duration, 0, rounds)
	for round := range rounds {
		entries := make([]segment.Entry, batchSize)
		for i := range entries {
			n := round*batchSize + i
			entries[i] = segment.Entry{
				Key:   []byte("batch-" + strconv.Itoa(n)),
				Value: make([]byte, 4096),
			}
		}
		start := time.Now()
		if err := e.Put(ctx, entries); err != nil {
			t.Fatal(err)
		}
		durs = append(durs, time.Since(start))
	}
	return buildLatencyStats(durs)
}

func buildLatencyStats(durs []time.Duration) latencyStats {
	if len(durs) == 0 {
		return latencyStats{}
	}
	sorted := append([]time.Duration(nil), durs...)
	slices.Sort(sorted)
	var total time.Duration
	for _, dur := range durs {
		total += dur
	}
	return latencyStats{
		avg: total / time.Duration(len(durs)),
		p50: sorted[len(sorted)/2],
		p95: sorted[(len(sorted)-1)*95/100],
		max: sorted[len(sorted)-1],
	}
}
