//go:build darwin || linux || windows

package packmove_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	block_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/s4wave/spacewave/db/volume/packmove"
	"github.com/sirupsen/logrus"
)

// packsBeforeFailure is the packs TestPackResume lets the bucket accept before
// it fails; each pack is two objects, the pack and its entry.
const packsBeforeFailure = 3

// packPrefix is the key prefix of the packs in the test bucket.
const packPrefix = "world/packs/"

// newPackStore builds a pack store on the test bucket that keeps its packs.
func newPackStore(client *block_store_s3.Client) *block_store_s3.PackStore {
	return block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), client, s3test.BucketName, packPrefix, &block_store_s3.PackStoreOptions{KeepPacks: true})
}

// packOnce packs the file at path with a read-only handle.
func packOnce(ctx context.Context, t *testing.T, path, logPath string, packer packmove.BlockPacker, tune func(*packmove.PackConfig)) (packmove.PackProgress, error) {
	// Open the file beside its writer.
	t.Helper()
	db, err := s4db.Open(path, s4db.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Pack small batches, so a run has many.
	cfg := packmove.PackConfig{
		Source:       db,
		Keys:         newKeys(t),
		WorldStoreID: worldStoreID,
		Packer:       packer,
		LogPath:      logPath,
		PackBytes:    8 * blockSize,
	}
	if tune != nil {
		tune(&cfg)
	}
	return packmove.Pack(ctx, cfg)
}

// switchOnce copies the stopped file at src to a new file at dst.
func switchOnce(ctx context.Context, t *testing.T, src, dst, logPath string) packmove.SwitchResult {
	// Open the source, which no other process may hold.
	t.Helper()
	sdb, err := s4db.Open(src, s4db.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer sdb.Close()
	if shared, err := sdb.Shared(); err != nil || shared {
		t.Fatalf("source shared %v: %v", shared, err)
	}

	// Create the new file.
	ddb, err := s4db.Open(dst, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer ddb.Close()

	// Switch.
	keys := newKeys(t)
	result, err := packmove.Switch(ctx, packmove.SwitchConfig{
		Source:          sdb,
		Dest:            ddb,
		Keys:            keys,
		LogPaths:        []string{logPath},
		WorldStoreID:    worldStoreID,
		WritebackPrefix: keys.GetObjectStorePrefixByID(writebackStoreID()),
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// fileSum returns the SHA-256 of the file at path.
func fileSum(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

// blockSource reads a block from a volume, then from the packs.
type blockSource struct {
	// local is the new volume.
	local *fixture
	// packs is the bucket's pack store.
	packs *block_store_s3.PackStore
}

// get reads the block, reporting whether the volume held it.
func (s *blockSource) get(ctx context.Context, ref *block.BlockRef) (data []byte, local bool, err error) {
	// Prefer the volume.
	data, found, err := s.local.vol.GetBlock(ctx, ref)
	if err != nil || found {
		return data, found, err
	}
	data, found, err = s.packs.GetBlock(ctx, ref)
	if err == nil && !found {
		err = errors.Errorf("block %s is in neither the volume nor the packs", ref.MarshalString())
	}
	return data, false, err
}

// TestPackSwitch packs a volume file while another process writes it, switches
// to a new file, and reads every block through the new volume and the packs.
func TestPackSwitch(t *testing.T) {
	// Lay out the files and the test bucket.
	ctx := t.Context()
	dir := t.TempDir()
	src, dst, logPath := filepath.Join(dir, "old.s4db"), filepath.Join(dir, "new.s4db"), filepath.Join(dir, "pack.log")
	bucket, client := s3test.New(t)
	packs := newPackStore(client)
	defer packs.Close()

	// Fill the file: World blocks with references, a block of another store,
	// a block that fails its hash, and the source's old upload records.
	f := openFixture(t, src, s4db.Options{})
	world := f.putWorld("base", 150)
	settings := f.put(f.other, "settings")
	rotted := f.putBad("rotted")
	f.putOldRecord(world[0])
	f.putOldRecord(rotted)
	f.close()

	// Pack while another process holds the file and writes it.
	w := startWriterProcess(ctx, t, src)
	progress, err := packOnce(ctx, t, src, logPath, packs, func(c *packmove.PackConfig) { c.BytesPerSecond = 64 << 10 })
	if err != nil {
		t.Fatal(err)
	}
	if progress.Bad != 1 || progress.Outside != 1 {
		t.Fatalf("bad %d outside %d, want 1 and 1", progress.Bad, progress.Outside)
	}
	if progress.Rest.Keys == 0 || !progress.Rest.Done {
		t.Fatalf("rest not measured: %+v", progress.Rest)
	}
	if progress.Blocks < uint64(len(world)) {
		t.Fatalf("packed %d blocks, want at least %d", progress.Blocks, len(world))
	}

	// Blocks written after the pack finished are for the switch to find.
	w.waitWrites(t, 3)
	written := w.stop(t)
	late := writerRefs(t, written)

	// Switch with every process closed; the source stays as it was.
	before := fileSum(t, src)
	result := switchOnce(ctx, t, src, dst, logPath)
	if fileSum(t, src) != before {
		t.Fatal("switch wrote the source")
	}
	if result.Bad != 1 || result.Outside != 1 || result.Unpacked < 3 {
		t.Fatalf("switch result %+v", result)
	}
	if result.Copy.DroppedBlocks != progress.Blocks || progress.Blocks+result.Unpacked != uint64(len(world)+written) {
		t.Fatalf("dropped %d blocks, packed %d, unpacked %d, of %d", result.Copy.DroppedBlocks, progress.Blocks, result.Unpacked, len(world)+written)
	}

	// Report how the writer's blocks split between the pack and the switch.
	t.Logf("the writer added %d blocks; the pack took %d blocks, the switch queued %d", written, progress.Blocks-uint64(len(world)), result.Unpacked)

	// Every block reads from the new volume or the packs.
	nf := openFixture(t, dst, s4db.Options{})
	defer nf.close()
	reads := &blockSource{local: nf, packs: packs}
	local := map[string]bool{}
	for name, ref := range map[string]*block.BlockRef{"settings": settings} {
		data, isLocal, err := reads.get(ctx, ref)
		if err != nil || !bytes.Equal(data, blockData(name)) || !isLocal {
			t.Fatalf("settings block: local %v err %v", isLocal, err)
		}
	}
	for i, ref := range append(append([]*block.BlockRef{}, world...), late...) {
		name := "base-" + itoa(i)
		if i >= len(world) {
			name = writerPrefix + "-" + itoa(i-len(world))
		}
		data, isLocal, err := reads.get(ctx, ref)
		if err != nil || !bytes.Equal(data, blockData(name)) {
			t.Fatalf("block %s: err %v", name, err)
		}
		local[ref.MarshalString()] = isLocal
	}
	if n := countTrue(local); uint64(n) != result.Unpacked {
		t.Fatalf("%d World blocks stayed in the volume, switch queued %d", n, result.Unpacked)
	}
	if exists, err := packs.GetBlockExists(ctx, settings); err != nil || exists {
		t.Fatalf("the other store's block is in a pack: %v %v", exists, err)
	}

	// The rotted block stays, unmarked, and out of the packs.
	rottedKey, err := rotted.MarshalKey()
	if err != nil {
		t.Fatal(err)
	}
	if data, found := rawGet(ctx, t, nf, nf.keys.GetBlockKey(rottedKey)); !found || string(data) != "rotted" {
		t.Fatalf("rotted block: %q %v", data, found)
	}
	if exists, err := packs.GetBlockExists(ctx, rotted); err != nil || exists {
		t.Fatalf("the rotted block is in a pack: %v %v", exists, err)
	}

	// The new file holds a fresh marker for each unpacked block and none of the
	// source's records.
	prefix := nf.keys.GetObjectStorePrefixByID(writebackStoreID())
	var records int
	tx, err := nf.db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		records++
		if bytes.Contains(key, []byte(world[0].GetHash().MarshalString())) {
			return errors.New("the source's record for a packed block survived")
		}
		return nil
	})
	tx.Discard()
	if err != nil {
		t.Fatal(err)
	}
	if records != 2*int(result.Unpacked) {
		t.Fatalf("%d records under the write-back prefix, want %d", records, 2*result.Unpacked)
	}

	// The write-back store reads the markers and uploads the unpacked blocks.
	wb, err := block_writeback.NewStore(ctx, nf.world, nf.vol, kvtx_prefixer.NewPrefixer(nf.db, prefix), writebackStoreID(), true)
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := wb.GetStatus(); uint64(status.Pending) != result.Unpacked {
		t.Fatalf("write-back sees %d pending blocks, want %d", status.Pending, result.Unpacked)
	}

	// Upload until nothing is pending.
	uploadCtx, stopUpload := context.WithCancel(ctx)
	uploaded := make(chan error, 1)
	go func() { uploaded <- wb.Upload(uploadCtx, packs) }()
	for {
		status, changed := wb.GetStatus()
		if status.Pending == 0 {
			break
		}
		select {
		case <-changed:
		case err := <-uploaded:
			t.Fatalf("upload stopped with %d blocks pending: %v", status.Pending, err)
		case <-time.After(30 * time.Second):
			t.Fatalf("upload left %d blocks pending", status.Pending)
		}
	}
	stopUpload()
	if err := <-uploaded; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, ref := range late {
		if exists, err := packs.GetBlockExists(ctx, ref); err != nil || !exists {
			t.Fatalf("block %s not uploaded: %v %v", ref.MarshalString(), exists, err)
		}
	}
	if len(bucket.Keys(packPrefix)) == 0 {
		t.Fatal("bucket holds no packs")
	}
}

// TestPackResume stops a pack on a failing bucket and between an upload and
// its commit, resumes it, and expects every block in exactly one logged pack.
// The pack uploaded before the stop was never logged, since uploads run before
// the log names their pack, so it stays in the bucket as one orphan.
func TestPackResume(t *testing.T) {
	// Lay out the files and the test bucket.
	ctx := t.Context()
	dir := t.TempDir()
	src, dst, logPath := filepath.Join(dir, "old.s4db"), filepath.Join(dir, "new.s4db"), filepath.Join(dir, "pack.log")
	bucket, client := s3test.New(t)
	packs := newPackStore(client)
	defer packs.Close()

	// Fill the file with World blocks.
	f := openFixture(t, src, s4db.Options{})
	world := f.putWorld("base", 100)
	f.close()

	// The bucket fails after three packs.
	accepted := 0
	bucket.FailPut(func(string) bool { accepted++; return accepted > 2*packsBeforeFailure })
	first, err := packOnce(ctx, t, src, logPath, packs, nil)
	if err == nil || first.Packs != packsBeforeFailure {
		t.Fatalf("first run: %d packs, err %v", first.Packs, err)
	}
	bucket.FailPut(nil)

	// The next run uploads a pack and stops before its commit.
	second, err := packOnce(ctx, t, src, logPath, &stopAfterPut{PackStore: packs}, nil)
	if err == nil || second.Packs != packsBeforeFailure {
		t.Fatalf("second run: %d packs, err %v", second.Packs, err)
	}

	// The third run packs those blocks again, and ends.
	third, err := packOnce(ctx, t, src, logPath, packs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if third.Blocks != uint64(len(world)) || third.Resumed == 0 || third.Bad != 0 {
		t.Fatalf("third run: %+v", third)
	}
	entries := uint64(len(bucket.Keys(packPrefix + "entries/")))
	if entries != third.Packs+1 || uint64(bucket.Puts()) != 2*(third.Packs+1) {
		t.Fatalf("bucket holds %d pack entries after %d puts, log counts %d packs", entries, bucket.Puts(), third.Packs)
	}

	// A fresh run changes nothing.
	again, err := packOnce(ctx, t, src, logPath, packs, nil)
	if err != nil || again.Packs != third.Packs || uint64(bucket.Puts()) != 2*(third.Packs+1) {
		t.Fatalf("rerun: %+v err %v puts %d", again, err, bucket.Puts())
	}

	// The switch drops every packed block and queues none.
	result := switchOnce(ctx, t, src, dst, logPath)
	if result.Unpacked != 0 || result.Copy.DroppedBlocks != uint64(len(world)) {
		t.Fatalf("switch result %+v", result)
	}
	nf := openFixture(t, dst, s4db.Options{})
	defer nf.close()
	reads := &blockSource{local: nf, packs: newPackStore(client)}
	for i, ref := range world {
		data, isLocal, err := reads.get(ctx, ref)
		if err != nil || isLocal || !bytes.Equal(data, blockData("base-"+itoa(i))) {
			t.Fatalf("block %d: local %v err %v", i, isLocal, err)
		}
	}
}

// stopAfterPut writes one batch to the pack store and then fails, as a pack
// killed between its upload and its commit.
type stopAfterPut struct {
	*block_store_s3.PackStore
	// stopped reports that the batch was written.
	stopped bool
}

// PutBlockBatch writes the first batch, then reports a failure.
func (s *stopAfterPut) PutBlockBatch(ctx context.Context, batch []*block.PutBatchEntry) ([]bool, error) {
	if s.stopped {
		return nil, errors.New("killed")
	}
	s.stopped = true
	if _, err := s.PackStore.PutBlockBatch(ctx, batch); err != nil {
		return nil, err
	}
	return nil, errors.New("killed after the upload")
}

// rawGet reads key straight from the fixture's volume file.
func rawGet(ctx context.Context, t *testing.T, f *fixture, key []byte) ([]byte, bool) {
	// Read in one snapshot.
	t.Helper()
	tx, err := f.db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	return data, found
}
