package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_packsweep_admin "github.com/s4wave/spacewave/db/block/packsweep/admin"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/block/writelog"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/sirupsen/logrus"
)

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
	for _, kind := range []string{packcopy.IndexSQLite, packcopy.IndexBolt, packcopy.IndexS4DB} {
		t.Run(kind, func(t *testing.T) { testCommandQuarantine(t, kind) })
	}
}

// testCommandQuarantine runs the quarantine with the pack index in a file of
// the volume type kind.
func testCommandQuarantine(t *testing.T, kind string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	dir := t.TempDir()

	// Write a live pack and three dead packs through an index file.
	const storeID = "p/local/acc/blk/so"
	_, client := s3test.New(t)
	indexPath := filepath.Join(dir, "index.db")
	openStore := func() (*block_store_s3.PackStore, func() error) {
		index, closeIndex, err := packcopy.OpenIndexFile(ctx, indexPath, &packcopy.IndexFileOptions{
			Kind:   kind,
			Table:  defaultIndexTable,
			SQLite: store_kvtx_sqlite.Pragmas{WithoutRowID: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return block_store_s3.NewPackStore(le, client, s3test.BucketName, "world/", &block_store_s3.PackStoreOptions{
			KeepPacks: true,
			Index:     kvtx_prefixer.NewPrefixer(index, block_store_s3.PackIndexPrefix(storeID)),
		}), closeIndex
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
		StoreID:    storeID,
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
	if err := args.runNewCycle(nil); err != nil {
		t.Fatal(err)
	}
	if err := args.runSnapshot(nil); err != nil {
		t.Fatal(err)
	}

	// Open the backend: the pack store, a marking store over it, the write
	// log and the admin service behind a credential.
	store, closeIndex := openStore()
	defer func() {
		store.Close()
		_ = closeIndex()
	}()
	marking := block_store_writeback.NewMarkingStore(store, nil)
	wlog := writelog.New(le)
	if err := wlog.Set(&writelog.Options{Dir: filepath.Join(dir, "log")}); err != nil {
		t.Fatal(err)
	}
	defer wlog.Close()
	server := block_packsweep_admin.NewServer(block_packsweep_admin.Options{
		Packs:   store,
		Log:     wlog,
		Enabled: func() bool { return true },
		Operations: func() (time.Time, time.Time) {
			ops := marking.OldestOperations()
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
	defer srv.Close()
	t.Setenv(adminAuthEnv, "Bearer admin")
	args.AdminURL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/admin/pack-sweep"

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
		_, err := client.GetObjectRange(ctx, s3test.BucketName, key, 0, 1)
		if found := err == nil; found != want || (err != nil && !errors.Is(err, block_store_s3.ErrNotFound)) {
			t.Fatalf("object %s found %v, %v; want %v", key, found, err, want)
		}
	}

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
}
