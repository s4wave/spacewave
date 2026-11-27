//go:build !js && !wasip1

package space_backup_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_gdrive "github.com/s4wave/spacewave/db/block/store/gdrive"
	"github.com/s4wave/spacewave/db/block/store/gdrive/gdrivetest"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/testbed"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
)

// measuredPass carries a complete pass result out of its background goroutine.
type measuredPass struct {
	// published is nil unless every reachable block was durable before its index.
	published *account_settings.SpaceBackupPublished
	// counts describes actual transferred bytes and packs.
	counts *account_settings.SpaceBackupProgress
	// err is the pass's terminal error.
	err error
}

// TestMeasureDriveBackupForeground measures durable World writes sharing the
// native source volume with a complete 4 GiB backup over real HTTP and disk.
// Its source root is retained by the fixture engine, independent of local pins.
func TestMeasureDriveBackupForeground(t *testing.T) {
	// Require an explicit disk directory; forge's /tmp may be backed by memory.
	if os.Getenv("SPACEWAVE_MEASURE_DRIVE_BACKUP") != "1" {
		t.Skip("set SPACEWAVE_MEASURE_DRIVE_BACKUP=1 on forge to measure")
	}
	base := os.Getenv("SPACEWAVE_MEASURE_DRIVE_BACKUP_DIR")
	if base == "" {
		t.Fatal("SPACEWAVE_MEASURE_DRIVE_BACKUP_DIR must name real disk")
	}
	directory, err := os.MkdirTemp(base, "drive-backup-measure-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})

	// Populate unique LFS files with bounded buffers, then encrypt through UnixFS.
	input := filepath.Join(directory, "input")
	writeMeasureFiles(t, input)
	fixture := buildBackupFixture(t, os.DirFS(input), testbed.WithVolumeConfig(&volume_s4db.Config{
		Path: filepath.Join(directory, "source.s4wave"), CacheBytes: 128 << 20,
	}))
	baseline := &writeLatency{}
	for range 1000 {
		baseline.add(measureWorldWrite(t, fixture))
	}

	// Store fake Drive's completed objects on disk rather than collecting 4 GiB.
	server, err := gdrivetest.NewDiskServer(filepath.Join(directory, "drive"))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root", server.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	volume, ok := fixture.source.(volume_kvtx.KvtxVolume)
	if !ok {
		t.Fatal("measurement requires a native transactional volume")
	}
	local := kvtx_prefixer.NewPrefixer(volume.GetKvtxStore(), []byte("measure-backup/"))
	pass := newPass(t, fixture, client, local, "backend/root/space/")
	finished := make(chan measuredPass, 1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := time.Now()
	go func() {
		published, counts, err := pass.Run(ctx, nil)
		finished <- measuredPass{published: published, counts: counts, err: err}
	}()

	// Measure foreground transactions until the complete pass reports its result.
	loaded := &writeLatency{}
	var result measuredPass
writing:
	for {
		select {
		case result = <-finished:
			break writing
		default:
			loaded.add(measureWorldWrite(t, fixture))
		}
	}
	if result.err != nil || result.published == nil || result.counts.GetWrittenBytes() < 4<<30 {
		t.Fatalf("4 GiB pass incomplete: %v, %v", result.counts, result.err)
	}
	if loaded.count < 1000 {
		t.Fatalf("only %d concurrent foreground samples; at least 1000 required", loaded.count)
	}
	t.Logf("4 GiB native source + disk-backed HTTP Drive: elapsed=%s transferred=%d packs=%d", time.Since(started), result.counts.GetWrittenBytes(), result.counts.GetPacks())
	t.Logf("baseline: n=%d p99<=%s max=%s; backup: n=%d p99<=%s max=%s", baseline.count, baseline.p99(), baseline.longest, loaded.count, loaded.p99(), loaded.longest)
	assertNoDelete(t, server)
}

// writeMeasureFiles streams 32 distinct 128 MiB files, keeping only a hash and
// io.Copy's fixed buffer, so neither deduplication nor RAM substitutes for disk.
func writeMeasureFiles(t *testing.T, directory string) {
	// Create one file at a time and durably rename it to its LFS content hash.
	t.Helper()
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for range 32 {
		file, err := os.CreateTemp(directory, "pending-")
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.New()
		_, copyErr := io.CopyN(io.MultiWriter(file, digest), rand.Reader, 128<<20)
		syncErr := file.Sync()
		closeErr := file.Close()
		if copyErr != nil || syncErr != nil || closeErr != nil {
			t.Fatalf("fixture file: copy=%v sync=%v close=%v", copyErr, syncErr, closeErr)
		}
		if err := os.Rename(file.Name(), filepath.Join(directory, hex.EncodeToString(digest.Sum(nil)))); err != nil {
			t.Fatal(err)
		}
	}
}

// measureWorldWrite commits one bounded foreground World mutation to the same
// disk volume the backup reads and writes its proofs to. The object count stays
// fixed by alternating one object's creation and deletion.
func measureWorldWrite(t *testing.T, fixture *backupFixture) time.Duration {
	// Include admission, reads, mutation, and durable commit in the measurement.
	t.Helper()
	ctx := t.Context()
	started := time.Now()
	tx, err := fixture.engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	found, err := tx.HasObject(ctx, "foreground-measure")
	if err == nil && found {
		_, err = tx.DeleteObject(ctx, "foreground-measure")
	} else if err == nil {
		var object world.ObjectState
		object, err = tx.CreateObject(ctx, "foreground-measure", nil)
		world.ReleaseObjectState(object)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return time.Since(started)
}
