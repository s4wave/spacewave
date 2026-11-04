//go:build linux

package boot

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// TestDataVolumeReopen exercises ordinary file Sync, abrupt process exit before
// Close, clean reopens, controller cancellation with live users and refused opens.
func TestDataVolumeReopen(t *testing.T) {
	// A child exits immediately after normal file Sync, bypassing every shutdown
	// fence. Reopen must therefore prove Sync, not the engine's Close flush.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	content := bytes.Repeat([]byte("durable file record\n"), 4096)
	if dir := os.Getenv("SPACEWAVE_OS_DATA_SYNC_CHILD"); dir != "" {
		d, err := CreateData(ctx, le, dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeImageFile(ctx, d.Handle(), "acknowledged", bytes.NewReader(content)); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}

	// Existing-open must not create even an empty database or directory.
	dir := filepath.Join(t.TempDir(), "data")
	if d, err := OpenData(ctx, le, dir); err == nil || d != nil {
		t.Fatal("existing-open accepted an absent fixture")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("existing-open changed absent directory: %v", err)
	}
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDataVolumeReopen$", "-test.timeout=20s")
	cmd.Env = append(os.Environ(), "SPACEWAVE_OS_DATA_SYNC_CHILD="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("file-sync child: %v\n%s", err, out)
	}

	// Reopen after the abrupt exit, read every accepted byte, then add another
	// record through the same production writer and cleanly close it.
	d, err := OpenData(ctx, le, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if d != nil {
			if err := d.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	id := d.volume.GetID()
	if err := verifyImageFile(ctx, d.Handle(), "acknowledged", bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	if err := writeImageFile(ctx, d.Handle(), "clean", bytes.NewBufferString("clean restart")); err != nil {
		t.Fatal(err)
	}

	// Keep a resolved child across healthy close, as FUSE can omit Forget.
	cached, err := d.Handle().Lookup(ctx, "clean")
	if err != nil {
		t.Fatal(err)
	}
	defer cached.Release()
	cachedCursor, cachedOps, err := cached.GetOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if !cachedCursor.CheckReleased() || !cachedOps.CheckReleased() {
		t.Fatal("healthy Data.Close retained a cached child's cursor or storage")
	}

	// Exclusive creation cannot reset the existing filesystem.
	path := filepath.Join(dir, "data.s4wave")
	before := hashImageFile(t, path)
	if replacement, err := CreateData(ctx, le, dir); !os.IsExist(err) || replacement != nil {
		t.Fatalf("recreate returned %v", err)
	}
	if hashImageFile(t, path) != before {
		t.Fatal("recreate modified existing data")
	}

	// Caller cancellation cannot close retained storage or its filesystem cursor.
	openCtx, cancel := context.WithCancel(ctx)
	d, err = OpenData(openCtx, le, dir)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if d.volume.GetID() != id {
		t.Fatal("reopen replaced the Volume identity")
	}
	cancel()
	if err := verifyImageFile(ctx, d.Handle(), "acknowledged", bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	if err := verifyImageFile(ctx, d.Handle(), "clean", bytes.NewBufferString("clean restart")); err != nil {
		t.Fatal(err)
	}
	file, err := d.Handle().Lookup(ctx, "acknowledged")
	if err != nil {
		t.Fatal(err)
	}
	fileCursor, fileOps, err := file.GetOps(ctx)
	if err != nil {
		file.Release()
		t.Fatal(err)
	}
	if err := file.WriteAt(ctx, 0, []byte("unacknowledged dirty buffer"), time.Now()); err != nil {
		file.Release()
		t.Fatal(err)
	}

	// Remove the actual controllers, including Execute cancellation and Close,
	// while a filesystem user is still retained. A fresh raw database transaction
	// must work after both joins; cached filesystem reads alone would not prove it.
	for _, ctrl := range d.bus.GetControllers() {
		if _, ok := ctrl.(volume.Controller); ok {
			d.bus.RemoveController(ctrl)
		}
	}
	if err := d.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Volume controller stop: %v", err)
	}
	d.bus.RemoveController(d.engineController)
	raw := volume_s4db.GetDB(d.volume)
	tx, err := raw.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal("controller closed physical storage under a file user: ", err)
	}
	_, found, err := tx.Get(ctx, store_kvkey.NewDefaultKVKey().GetFormatVersionKey())
	tx.Discard()
	if err != nil || !found {
		t.Fatalf("physical read after controller cancellation: %v, %v", found, err)
	}

	// Closing with an idle dirty file still retained models a joined FUSE server
	// that omitted Forget. Close must interrupt buffered Sync and join cursors.
	if err := d.Close(); !errors.Is(err, context.Canceled) {
		file.Release()
		t.Fatalf("dirty close after controller stop: %v", err)
	}
	if !d.cursor.CheckReleased() || !fileCursor.CheckReleased() || !fileOps.CheckReleased() {
		file.Release()
		t.Fatal("Data.Close left filesystem watchers or cached child storage live")
	}
	file.Release()
	d = nil
	if unexpected, err := raw.NewTransaction(ctx, false); err == nil {
		unexpected.Discard()
		t.Fatal("Data.Close did not physically close storage")
	}

	// Refused opens preserve exact bytes for unknown formats, broken generated
	// heads and missing roots. Poison copies only; keep the good fixture intact.
	for _, poison := range []string{"unknown", "format", "head", "absent-head", "root"} {
		badDir := t.TempDir()
		badPath := filepath.Join(badDir, "data.s4wave")
		copyDataFile(t, path, badPath)
		poisonDataFile(t, le, badPath, poison)
		before := hashImageFile(t, badPath)
		if bad, err := OpenData(ctx, le, badDir); err == nil || bad != nil {
			t.Fatalf("%s store was accepted", poison)
		}
		if hashImageFile(t, badPath) != before {
			t.Fatalf("%s open modified rejected data", poison)
		}
	}
}

// copyDataFile streams a closed disposable fixture into a separate adversary.
func copyDataFile(t *testing.T, source, target string) {
	// Retain both files until the bounded streaming copy completes.
	t.Helper()
	src, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Report copy and close failures before the adversary opens the new file.
	_, copyErr := io.Copy(dst, src)
	if err := errors.Join(copyErr, dst.Close()); err != nil {
		t.Fatal(err)
	}
}

// poisonDataFile damages only a disposable copy through the real storage API.
func poisonDataFile(t *testing.T, le *logrus.Entry, path, poison string) {
	// An unknown physical file must never reach a writable Volume constructor.
	t.Helper()
	if poison == "unknown" {
		if err := os.WriteFile(path, []byte("unknown store"), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}

	// Open a valid copy before deliberately violating its format or head contract.
	ctx := t.Context()
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{
		Path: path, SingleProcess: true, NoGenerateKey: true, NoWriteKey: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := vol.Close(); err != nil {
			t.Error(err)
		}
	}()
	if poison == "format" {
		tx, err := vol.GetKvtxStore().NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		if err := tx.Set(ctx, vol.GetKvKey().GetFormatVersionKey(), []byte("unknown version")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return
	}

	// Bypass atomic publication only to construct corrupt generated metadata.
	store, release, err := vol.AccessObjectStore(ctx, dataHeadStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	data := []byte{0xff}
	if poison == "root" {
		missing, err := block.BuildBlockRef([]byte("absent data root"), nil)
		if err != nil {
			t.Fatal(err)
		}
		data, err = (&world_block_engine.HeadState{
			HeadRef: &bucket.ObjectRef{BucketId: dataBucket, RootRef: missing},
		}).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
	}
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if poison == "absent-head" {
		err = tx.Delete(ctx, []byte(dataHeadKey))
	} else {
		err = tx.Set(ctx, []byte(dataHeadKey), data)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
