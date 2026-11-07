package main

import (
	"bytes"
	"context"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_packsweep_admin "github.com/s4wave/spacewave/db/block/packsweep/admin"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_s3_overlay "github.com/s4wave/spacewave/db/block/store/s3/overlay"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/block/writelog"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/sirupsen/logrus"
)

// commandStoreID is the block store id of the command tests' pack index.
const commandStoreID = "p/local/acc/blk/so"

// openIndexStore opens a pack store over the bucket of client, its pack index
// in the file at path of the volume type kind.
func openIndexStore(ctx context.Context, t *testing.T, kind, path string, client block_store_s3.ObjectStore) (*block_store_s3.PackStore, func() error) {
	t.Helper()
	store, _, closeIndex := openIndexFile(ctx, t, kind, path, client)
	return store, closeIndex
}

// openIndexFile opens a pack store as openIndexStore does, and returns the
// index file's key space too.
func openIndexFile(ctx context.Context, t *testing.T, kind, path string, client block_store_s3.ObjectStore) (*block_store_s3.PackStore, kvtx.Store, func() error) {
	t.Helper()
	index, closeIndex, err := packcopy.OpenIndexFile(ctx, path, &packcopy.IndexFileOptions{
		Kind:   kind,
		Table:  defaultIndexTable,
		SQLite: store_kvtx_sqlite.Pragmas{WithoutRowID: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), client, s3test.BucketName, "world/", &block_store_s3.PackStoreOptions{
		KeepPacks: true,
		Index:     kvtx_prefixer.NewPrefixer(index, block_store_s3.PackIndexPrefix(commandStoreID)),
	}), index, closeIndex
}

// commandBackend is a backend for the command tests: a pack store, a marking
// store over it, the write log, and the admin service behind a credential.
type commandBackend struct {
	// store is the pack store.
	store *block_store_s3.PackStore
	// marking records the open operations over store.
	marking *block_store_writeback.MarkingStore
	// wlog is the write log.
	wlog *writelog.Log
	// url is the admin service's ws URL.
	url string
	// stop stops the admin service and closes the write log, once.
	stop func()
}

// serveAdmin serves the admin service over store until the test ends, and
// sets the credential the command sends.
func serveAdmin(t *testing.T, store *block_store_s3.PackStore, dir string) *commandBackend {
	t.Helper()
	b := &commandBackend{store: store, marking: block_store_writeback.NewMarkingStore(store, func(context.Context, []block_store_writeback.Mark) error { return nil }), wlog: writelog.New(logrus.NewEntry(logrus.New()))}
	if err := b.wlog.Set(&writelog.Options{Dir: filepath.Join(dir, "log")}); err != nil {
		t.Fatal(err)
	}
	server := block_packsweep_admin.NewServer(block_packsweep_admin.Options{
		Packs:   store,
		Log:     b.wlog,
		Refs:    packsweep.SourceRefs{store},
		Enabled: func() bool { return true },
		Operations: func() (time.Time, time.Time) {
			ops := b.marking.OldestOperations()
			return ops.Write, ops.Read
		},
	})
	handler, err := block_packsweep_admin.NewHTTPHandler(server, "/admin/pack-sweep")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	b.stop = sync.OnceFunc(func() {
		srv.Close()
		_ = b.wlog.Close()
	})
	t.Cleanup(b.stop)
	t.Setenv(adminAuthEnv, "Bearer admin")
	b.url = "ws" + strings.TrimPrefix(srv.URL, "http") + "/admin/pack-sweep"
	return b
}

// rehearse, when on, names an overlay on args and returns one over client for
// the backend, with a check that the store is as it was when rehearse was
// called. Off, it returns client and a check of nothing.
func rehearse(t *testing.T, on bool, args *Args, client *block_store_s3.Client, dir string) (block_store_s3.ObjectStore, func()) {
	// Lay the overlay over the store.
	t.Helper()
	if !on {
		return client, func() {}
	}
	args.Overlay = filepath.Join(dir, "overlay")
	objects, err := block_store_s3_overlay.New(client, s3test.BucketName, args.Overlay)
	if err != nil {
		t.Fatal(err)
	}

	// Check the store's objects are as they were.
	before := readObjects(t, client)
	return objects, func() {
		t.Helper()
		after := readObjects(t, client)
		if !maps.EqualFunc(before, after, bytes.Equal) {
			t.Fatalf("the rehearsal changed the store: %v before, %v after", slices.Sorted(maps.Keys(before)), slices.Sorted(maps.Keys(after)))
		}
	}
}

// readObjects reads every object of client's bucket.
func readObjects(t *testing.T, client block_store_s3.ObjectStore) map[string][]byte {
	t.Helper()
	ctx := context.Background()
	out := make(map[string][]byte)
	err := client.ListObjects(ctx, s3test.BucketName, "", func(key string, _ int64) error {
		rc, err := client.GetObject(ctx, s3test.BucketName, key)
		if err != nil {
			return err
		}
		out[key], err = io.ReadAll(rc)
		_ = rc.Close()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// commandRuns are the command tests' runs: each volume type of the pack
// index, on the store itself and as a rehearsal through an overlay.
func commandRuns(t *testing.T, run func(t *testing.T, kind string, rehearsal bool)) {
	for _, kind := range []string{packcopy.IndexSQLite, packcopy.IndexBolt, packcopy.IndexS4DB} {
		t.Run(kind, func(t *testing.T) { run(t, kind, false) })
		t.Run(kind+"/overlay", func(t *testing.T) { run(t, kind, true) })
	}
}

// TestCommandQuarantine quarantines, rechecks, trashes and removes through a
// backend's admin service over HTTP, its pack store on an index file of each
// volume type and its open operations from a marking store over it. The
// recheck holds the two dead packs while young, then while a read operation
// open since before their quarantine remains, then lifts the one a put
// relinked and passes the other. The trash moves the passed pack's objects to
// the store's trash, and the removal points the row of a block it shares with
// the root's pack at that pack. A read that misses a third dead pack in the
// trash keeps it from the removal, and the restore returns it to the reads; a
// restore naming the removed pack adopts it back.
func TestCommandQuarantine(t *testing.T) {
	commandRuns(t, testCommandQuarantine)
}

// testCommandQuarantine runs the quarantine with the pack index in a file of
// the volume type kind, as a rehearsal when rehearsal is set.
func testCommandQuarantine(t *testing.T, kind string, rehearsal bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := t.TempDir()

	// Write a live pack and three dead packs through an index file.
	_, client := s3test.New(t)
	indexPath := filepath.Join(dir, "index.db")
	var objects block_store_s3.ObjectStore = client
	openStore := func() (*block_store_s3.PackStore, func() error) {
		return openIndexStore(ctx, t, kind, indexPath, objects)
	}
	ref := func(data string) *block.BlockRef {
		r, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	rootRef, sharedRef, deadRef, relinkRef, lostRef := ref("root"), ref("shared"), ref("dead"), ref("relink"), ref("lost")
	seed, closeIndex := openStore()
	entry := func(r *block.BlockRef, data string) *block.PutBatchEntry {
		return &block.PutBatchEntry{Ref: r, Data: []byte(data), RefsKnown: true}
	}
	for _, batch := range [][]*block.PutBatchEntry{
		{entry(rootRef, "root"), entry(sharedRef, "shared")},
		{entry(deadRef, "dead"), entry(sharedRef, "shared")},
		{entry(relinkRef, "relink")},
		{entry(lostRef, "lost")},
	} {
		if _, err := seed.PutBlockBatch(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	seed.Close()
	if err := closeIndex(); err != nil {
		t.Fatal(err)
	}

	// Snapshot the store from its index file while no backend holds it.
	var out bytes.Buffer
	args := &Args{
		Scratch:    filepath.Join(dir, "scratch"),
		StoreID:    commandStoreID,
		Index:      indexPath,
		Prefix:     "world/",
		Reads:      2,
		Roots:      *cli.NewStringSlice(rootRef.MarshalString()),
		Batch:      1,
		MinAge:     time.Nanosecond,
		LowPercent: packsweep.DefaultLowLivePercent,
		Report:     filepath.Join(dir, "report.jsonl"),
		Holes:      filepath.Join(dir, "holes.txt"),
		dial: func() (block_store_s3.ObjectStore, string, func(), error) {
			return client, s3test.BucketName, func() {}, nil
		},
		out: &out,
	}
	objects, unchanged := rehearse(t, rehearsal, args, client, dir)
	if err := args.runNewCycle(nil); err != nil {
		t.Fatal(err)
	}
	if err := args.runSnapshot(nil); err != nil {
		t.Fatal(err)
	}

	// Open the backend over the index file.
	store, closeIndex := openStore()
	defer func() {
		store.Close()
		_ = closeIndex()
	}()
	backend := serveAdmin(t, store, dir)
	marking, wlog := backend.marking, backend.wlog
	args.AdminURL = backend.url

	// Walk from the log's position, and judge the three packs dead. The judgment
	// waits on the unread log, which the quarantine pulls.
	pos, ok := wlog.Position()
	if !ok {
		t.Fatal("write log is off")
	}
	args.LogPosition = pos.String()
	if err := args.runWalk(nil); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := args.runJudge(nil); err != nil {
		t.Fatal(err)
	}
	if result := parse(t, out.Bytes()); result.GetUint("summary", "packs", "dead") != 3 || result.GetUint("blocking", packsweep.BlockLogUnread) != 1 {
		t.Fatalf("judgment = %s", result)
	}

	locatedBefore, err := store.LocatePacks(ctx, []*block.BlockRef{deadRef, sharedRef})
	if err != nil || locatedBefore[0] == "" || locatedBefore[1] != locatedBefore[0] {
		t.Fatalf("locate before the quarantine = %v, %v", locatedBefore, err)
	}

	// A read operation open from before the quarantine, then the quarantine
	// holds the packs out of the reads.
	_, release, err := marking.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	out.Reset()
	if err := args.runQuarantine(nil); err != nil {
		t.Fatal(err)
	}
	if res := parse(t, out.Bytes()); res.GetUint("quarantined") != 3 || res.GetUint("live") != 0 || string(res.GetStringBytes("blocked")) != "" {
		t.Fatalf("quarantine = %s", res)
	}
	located, err := store.LocatePacks(ctx, []*block.BlockRef{deadRef, relinkRef, rootRef})
	if err != nil || located[0] != "" || located[1] != "" || located[2] == "" {
		t.Fatalf("locate after the quarantine = %v, %v", located, err)
	}

	// The recheck holds them while young, then while the read is open.
	recheck := func(wait time.Duration) string {
		t.Helper()
		args.Wait = wait
		out.Reset()
		if err := args.runRecheck(nil); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	if res := parse(t, []byte(recheck(time.Hour))); res.GetUint("waiting", packsweep.WaitQuarantine) != 3 {
		t.Fatalf("young recheck = %s", res)
	}
	if res := parse(t, []byte(recheck(time.Nanosecond))); res.GetUint("waiting", packsweep.WaitOpenRead) != 3 {
		t.Fatalf("recheck with the read open = %s", res)
	}

	// Once the read is released, a put relinking one pack lifts it, and the
	// others pass.
	release()
	wlog.Put(relinkRef.GetHash())
	if res := parse(t, []byte(recheck(time.Nanosecond))); res.GetUint("passed") != 2 || res.GetUint("live") != 1 || res.Get("waiting", packsweep.WaitOpenRead) != nil {
		t.Fatalf("recheck = %s", res)
	}
	located, err = store.LocatePacks(ctx, []*block.BlockRef{deadRef, relinkRef})
	if err != nil || located[0] != "" || located[1] == "" {
		t.Fatalf("locate after the recheck = %v, %v", located, err)
	}

	// The trash moves the passed packs' entries and files to the store's
	// trash, refusing a scratch file of another prefix, and a repeat moves
	// nothing.
	deadPack := locatedBefore[0]
	args.Prefix = "other/"
	if err := args.runTrash(nil); err == nil {
		t.Fatal("trashed through a scratch file of another prefix")
	}
	args.Prefix = "world/"
	for _, want := range []uint{2, 0} {
		out.Reset()
		if err := args.runTrash(nil); err != nil {
			t.Fatal(err)
		}
		if res := parse(t, out.Bytes()); res.GetUint("moved") != want || res.GetUint("missed") != 0 {
			t.Fatalf("trash = %s", res)
		}
	}
	for key, want := range map[string]bool{
		block_store_s3.EntryObjectKey("world/", deadPack): false,
		block_store_s3.PackObjectKey("world/", deadPack):  false,
		block_store_s3.TrashEntryKey("world/", deadPack):  true,
		block_store_s3.TrashPackKey("world/", deadPack):   true,
	} {
		_, err := objects.GetObjectRange(ctx, s3test.BucketName, key, 0, 1)
		if found := err == nil; found != want || (err != nil && !errors.Is(err, block_store_s3.ErrNotFound)) {
			t.Fatalf("object %s found %v, %v; want %v", key, found, err, want)
		}
	}
	unchanged()

	// A read of the lost block finds its pack in the trash and records the
	// miss.
	if _, _, err := store.GetBlock(ctx, lostRef); !errors.Is(err, block_store_s3.ErrNotFound) {
		t.Fatalf("read of the trashed lost block = %v, want not found", err)
	}

	// The removal drops the dead pack, pointing the shared block's row at
	// the root's pack, keeps the missed one, and a repeat removes nothing.
	for _, want := range []uint{1, 0} {
		out.Reset()
		if err := args.runRemove(nil); err != nil {
			t.Fatal(err)
		}
		if res := parse(t, out.Bytes()); res.GetUint("removed") != want || res.GetUint("missed") != 1 {
			t.Fatalf("removal = %s", res)
		}
	}
	located, err = store.LocatePacks(ctx, []*block.BlockRef{sharedRef, rootRef, deadRef})
	if err != nil || located[0] == "" || located[0] != located[1] || located[2] != "" {
		t.Fatalf("locate after the removal = %v, %v", located, err)
	}
	if data, found, err := store.GetBlock(ctx, sharedRef); err != nil || !found || string(data) != "shared" {
		t.Fatalf("read of the shared block = %q, %v, %v", data, found, err)
	}

	// The restore moves the missed pack back and registers it, the lost block
	// reads again, and a repeat restores nothing.
	for _, want := range []uint{1, 0} {
		out.Reset()
		if err := args.runRestore(nil); err != nil {
			t.Fatal(err)
		}
		if res := parse(t, out.Bytes()); res.GetUint("restored") != want || res.GetUint("dropped") != 0 {
			t.Fatalf("restore = %s", res)
		}
	}
	if data, found, err := store.GetBlock(ctx, lostRef); err != nil || !found || string(data) != "lost" {
		t.Fatalf("read of the restored lost block = %q, %v, %v", data, found, err)
	}
	if located, err := store.LocatePacks(ctx, []*block.BlockRef{lostRef}); err != nil || located[0] == "" {
		t.Fatalf("locate of the restored lost block = %v, %v", located, err)
	}

	// The status counts the records, and a dial without the credential fails.
	out.Reset()
	if err := args.runStatus(nil); err != nil {
		t.Fatal(err)
	}
	if status := parse(t, out.Bytes()); status.GetUint("packRecords", "removed") != 1 || status.GetUint("packRecords", "quarantine") != 0 || status.GetUint("packRecords", "moved") != 0 {
		t.Fatalf("status = %s", status)
	}
	// A restore naming the removed dead pack moves it back and adopts it:
	// its block reads again, and the status counts no removed pack.
	args.Packs = *cli.NewStringSlice(deadPack)
	out.Reset()
	if err := args.runRestore(nil); err != nil {
		t.Fatal(err)
	}
	if res := parse(t, out.Bytes()); res.GetUint("adopted") != 1 || res.GetUint("restored") != 0 {
		t.Fatalf("named restore = %s", res)
	}
	if data, found, err := store.GetBlock(ctx, deadRef); err != nil || !found || string(data) != "dead" {
		t.Fatalf("read of the adopted dead block = %q, %v, %v", data, found, err)
	}
	if located, err := store.LocatePacks(ctx, []*block.BlockRef{deadRef}); err != nil || located[0] != deadPack {
		t.Fatalf("locate of the adopted dead block = %v, %v", located, err)
	}
	out.Reset()
	if err := args.runStatus(nil); err != nil {
		t.Fatal(err)
	}
	if status := parse(t, out.Bytes()); status.GetUint("packRecords", "removed") != 0 {
		t.Fatalf("status after the adoption = %s", status)
	}
	if err := args.runRestore(nil); err == nil {
		t.Fatal("adopted the dead pack twice")
	}
	args.Packs = cli.StringSlice{}
	t.Setenv(adminAuthEnv, "")
	if err := args.runRecheck(nil); err == nil {
		t.Fatal("rechecked without the credential")
	}

	// A rehearsal left the store as it was, and its scratch file refuses the
	// store itself.
	unchanged()
	if rehearsal {
		args.Overlay = ""
		if err := args.runTrash(nil); err == nil || !strings.Contains(err.Error(), "overlay") {
			t.Fatalf("trash of the store through a rehearsal's scratch file = %v", err)
		}
	}
}

// TestCommandCompact compacts a low pack through a backend's admin service,
// its pack index in a file of each volume type. The quarantine leaves the low
// pack until the compaction writes its live block to a new pack the backend
// adopts and reads it from; then the quarantine, recheck, trash and removal
// take the low pack, and the live block still reads.
func TestCommandCompact(t *testing.T) {
	commandRuns(t, testCommandCompact)
}

// testCommandCompact runs the compaction with the pack index in a file of the
// volume type kind, as a rehearsal when rehearsal is set.
func testCommandCompact(t *testing.T, kind string, rehearsal bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := t.TempDir()

	// Write the root's pack, and a low pack holding the block it links and a
	// large dead block, through an index file.
	_, client := s3test.New(t)
	indexPath := filepath.Join(dir, "index.db")
	dead := strings.Repeat("dead ", 400)
	ref := func(data string) *block.BlockRef {
		r, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	rootRef, liveRef, deadRef := ref("root"), ref("live"), ref(dead)
	seed, closeIndex := openIndexStore(ctx, t, kind, indexPath, client)
	for _, batch := range [][]*block.PutBatchEntry{
		{{Ref: rootRef, Data: []byte("root"), Refs: []*block.BlockRef{liveRef}, RefsKnown: true}},
		{{Ref: liveRef, Data: []byte("live"), RefsKnown: true}, {Ref: deadRef, Data: []byte(dead), RefsKnown: true}},
	} {
		if _, err := seed.PutBlockBatch(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	seed.Close()
	if err := closeIndex(); err != nil {
		t.Fatal(err)
	}

	// Snapshot the store while no backend holds it, then open the backend.
	var out bytes.Buffer
	args := &Args{
		Scratch:      filepath.Join(dir, "scratch"),
		StoreID:      commandStoreID,
		Index:        indexPath,
		Prefix:       "world/",
		Reads:        2,
		Roots:        *cli.NewStringSlice(rootRef.MarshalString()),
		Batch:        1,
		MinAge:       time.Nanosecond,
		LowPercent:   packsweep.DefaultLowLivePercent,
		Report:       filepath.Join(dir, "report.jsonl"),
		Holes:        filepath.Join(dir, "holes.txt"),
		MaxPackBytes: 1 << 20,
		dial: func() (block_store_s3.ObjectStore, string, func(), error) {
			return client, s3test.BucketName, func() {}, nil
		},
		out: &out,
	}
	objects, unchanged := rehearse(t, rehearsal, args, client, dir)
	if err := args.runNewCycle(nil); err != nil {
		t.Fatal(err)
	}
	if err := args.runSnapshot(nil); err != nil {
		t.Fatal(err)
	}
	store, closeIndex := openIndexStore(ctx, t, kind, indexPath, objects)
	defer func() {
		store.Close()
		_ = closeIndex()
	}()
	backend := serveAdmin(t, store, dir)
	args.AdminURL = backend.url
	run := func(step func(*cli.Context) error) *fastjson.Value {
		t.Helper()
		out.Reset()
		if err := step(nil); err != nil {
			t.Fatal(err)
		}
		return parse(t, out.Bytes())
	}

	// Walk from the log's position, and judge the second pack low.
	pos, ok := backend.wlog.Position()
	if !ok {
		t.Fatal("write log is off")
	}
	args.LogPosition = pos.String()
	if err := args.runWalk(nil); err != nil {
		t.Fatal(err)
	}
	if res := run(args.runJudge); res.GetUint("summary", "packs", "low") != 1 {
		t.Fatalf("judgment = %s", res)
	}
	located, err := store.LocatePacks(ctx, []*block.BlockRef{liveRef, deadRef})
	if err != nil || located[0] == "" || located[1] != located[0] {
		t.Fatalf("locate before the compaction = %v, %v", located, err)
	}
	lowPack := located[0]

	// The quarantine leaves the low pack before the compaction.
	if res := run(args.runQuarantine); res.GetUint("unplaced") != 1 || res.GetUint("quarantined") != 0 {
		t.Fatalf("quarantine before the compaction = %s", res)
	}

	// The compaction writes the live block to a new pack the backend reads it
	// from, and a repeat writes nothing.
	if res := run(args.runCompact); res.GetUint("packs") != 1 || res.GetUint("blocks") != 1 || res.GetUint("victims") != 1 {
		t.Fatalf("compaction = %s", res)
	}
	if res := run(args.runCompact); res.GetUint("packs") != 0 || res.GetUint("victims") != 0 {
		t.Fatalf("second compaction = %s", res)
	}
	located, err = store.LocatePacks(ctx, []*block.BlockRef{liveRef, deadRef})
	if err != nil || located[0] == "" || located[0] == lowPack || located[1] != lowPack {
		t.Fatalf("locate after the compaction = %v, %v", located, err)
	}
	newPack := located[0]
	if status := run(args.runStatus); status.GetUint("compactPacks", "adopted") != 1 || status.GetUint("compactPacks", "pending") != 0 {
		t.Fatalf("status = %s", status)
	}

	// The quarantine, recheck, trash and removal take the low pack, and the
	// live block reads from the new pack throughout.
	readLive := func(step string) {
		t.Helper()
		if data, found, err := store.GetBlock(ctx, liveRef); err != nil || !found || string(data) != "live" {
			t.Fatalf("read of the live block after the %s = %q, %v, %v", step, data, found, err)
		}
		if located, err := store.LocatePacks(ctx, []*block.BlockRef{liveRef}); err != nil || located[0] != newPack {
			t.Fatalf("locate of the live block after the %s = %v, %v", step, located, err)
		}
	}
	if res := run(args.runQuarantine); res.GetUint("quarantined") != 1 || res.GetUint("unplaced") != 0 {
		t.Fatalf("quarantine after the compaction = %s", res)
	}
	readLive("quarantine")
	args.Wait = time.Nanosecond
	if res := run(args.runRecheck); res.GetUint("passed") != 1 {
		t.Fatalf("recheck = %s", res)
	}
	if res := run(args.runTrash); res.GetUint("moved") != 1 {
		t.Fatalf("trash = %s", res)
	}
	readLive("trash")
	if res := run(args.runRemove); res.GetUint("removed") != 1 {
		t.Fatalf("removal = %s", res)
	}
	readLive("removal")
	if located, err := store.LocatePacks(ctx, []*block.BlockRef{deadRef}); err != nil || located[0] != "" {
		t.Fatalf("locate of the dead block after the removal = %v, %v", located, err)
	}
	unchanged()
}
