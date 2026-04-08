//go:build js

package blockshard

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/volume/js/opfs/segment"
)

func newTestEngine(t *testing.T, dirName, lockPrefix string) (*Engine, func()) {
	return newTestEngineWithSettings(t, dirName, lockPrefix, nil)
}

func newTestEngineWithSettings(t *testing.T, dirName, lockPrefix string, settings *Settings) (*Engine, func()) {
	t.Helper()
	if (settings == nil || !settings.AsyncIO) && !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}
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

func publishEntries(t *testing.T, s *Shard, entries []segment.Entry) {
	t.Helper()
	release, err := s.AcquirePublishLock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := s.Publish(entries); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReclaimPendingDelete(); err != nil {
		t.Fatal(err)
	}
}

func compactShard(t *testing.T, s *Shard) {
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
	if dur >= 40*time.Millisecond {
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

	now = now.Add(DefaultRetireGracePeriod + time.Millisecond)
	for _, v := range []string{"v5", "v6"} {
		publishEntries(t, writer.shards[0], []segment.Entry{{
			Key:   key,
			Value: []byte(v),
		}})
	}

	missing := stale.Segments[len(stale.Segments)-1].Filename
	exists, err := opfs.FileExists(writer.shards[0].dir, missing)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatalf("expected reclaimed segment %q to be deleted", missing)
	}

	reader.shards[0].mu.Lock()
	reader.shards[0].manifest = stale.Clone()
	reader.shards[0].mu.Unlock()

	val, found, err := reader.GetFromShard(0, key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v6" {
		t.Fatalf("stale reader result: found=%v val=%q want v6", found, val)
	}
}
