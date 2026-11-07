package block_store_nodepush

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/sirupsen/logrus"
)

// sftpFake is the SFTP server beside the node: it holds the remote's packs,
// which the read copies come from, and takes the node's uploads.
type sftpFake struct {
	*objectRemote
	mtx sync.Mutex
	// uploads holds the packs the node's drain put, by ID.
	uploads map[string][]byte
}

// HasPackfile reports whether the node uploaded the pack.
func (s *sftpFake) HasPackfile(_ context.Context, id string) (bool, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	_, ok := s.uploads[id]
	return ok, nil
}

// PutPackFile takes an upload, which the remote then serves like any pack.
func (s *sftpFake) PutPackFile(_ context.Context, id string, data []byte) (*packfile.PackfileEntry, error) {
	s.mtx.Lock()
	s.uploads[id] = bytes.Clone(data)
	s.mtx.Unlock()
	s.set("p/packs/"+id, data)
	return &packfile.PackfileEntry{Id: id, SizeBytes: uint64(len(data))}, nil
}

// uploaded returns a copy of the uploads.
func (s *sftpFake) uploaded() map[string][]byte {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	out := make(map[string][]byte, len(s.uploads))
	for id, data := range s.uploads {
		out[id] = data
	}
	return out
}

// list walks the remote's packs with their sizes, as the SFTP listing does.
func (s *sftpFake) list(_ context.Context, fn func(id string, size int64) error) error {
	s.objectRemote.mtx.Lock()
	keys := make(map[string]int64, len(s.objects))
	for key, data := range s.objects {
		keys[key] = int64(len(data))
	}
	s.objectRemote.mtx.Unlock()
	for key, size := range keys {
		if err := fn(filepath.Base(key), size); err != nil {
			return err
		}
	}
	return nil
}

// fillMutantEnv marks the run of TestNodeFillsNeverUpload under a mutant.
const fillMutantEnv = "NODEPUSH_FILL_MUTANT"

// TestNodeFillsNeverUpload runs a node with pushes, misses, a backfill and a
// restart, and checks the uploader only ever sends what was pushed: a pack read
// from the remote is copied byte for byte under its own ID into the read
// copies, served from there, and never reaches the pack log or the upload.
// Pruning then removes read copies oldest first and never touches the log.
func TestNodeFillsNeverUpload(t *testing.T) {
	// The remote holds three packs only it has.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	remote := &sftpFake{objectRemote: &objectRemote{}, uploads: map[string][]byte{}}
	seeds := map[string][]byte{}
	var seedIDs []string
	for range 3 {
		id := identity.NewPackID()
		data := make([]byte, 64<<10)
		_, _ = rand.Read(data)
		remote.set("p/packs/"+id, data)
		seeds[id] = data
		seedIDs = append(seedIDs, id)
	}

	// A node with room to spare, whose free disk the test sets.
	dir := t.TempDir()
	var freeBase atomic.Int64
	freeBase.Store(1 << 40)
	var current atomic.Pointer[Node]
	open := func() *Node {
		t.Helper()
		n, err := OpenNode(ctx, logrus.NewEntry(logrus.New()), NodeConfig{
			Dir:            dir,
			Linger:         10 * time.Millisecond,
			Remote:         remote,
			Source:         remote,
			Bucket:         "bucket",
			Prefix:         "p/packs",
			List:           remote.list,
			RemoteReads:    8,
			MinFreeBytes:   100 << 10,
			PruneFreeBytes: 50 << 10,
			FreeBytes: func() (uint64, bool) {
				return uint64(freeBase.Load()) + current.Load().packs.Stats().PrunedBytes, true //nolint:gosec // the test's values are positive
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		current.Store(n)
		return n
	}
	node := open()
	runCtx, stopRun := context.WithCancel(ctx)
	ran := make(chan struct{})
	go func() { defer close(ran); node.Run(runCtx) }()
	apply := &applier{applied: map[string][][]byte{}, changed: make(chan struct{}, 1)}
	client := NewClient(logrus.NewEntry(logrus.New()), apply)
	end := connect(ctx, t, client, node.server)

	// Pushes reach the log and the upload.
	pushed := map[string]bool{}
	for i := range 4 {
		var batch []*block.PutBatchEntry
		for j := range 3 {
			e := entry(t, strings.Repeat("pushed", 10)+string(rune('a'+i))+string(rune('a'+j)))
			batch = append(batch, e)
			pushed[string(packfile.BlockKey(e.Ref.GetHash()))] = true
		}
		if err := client.Send(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}

	// Every push uploads, and the node keeps a read copy of each pack it
	// uploaded, the upload byte for byte.
	settled := func() {
		t.Helper()
		waitFor(t, func() bool {
			var keys int
			for _, data := range remote.uploaded() {
				index, err := packfile.ReadIndex(bytes.NewReader(data), uint64(len(data)))
				if err != nil {
					t.Fatal(err)
				}
				keys += len(index)
			}
			st := node.Stats()
			return keys >= len(pushed) && st.Upload.Pending == 0 && st.Log.Packs == st.Log.Finalized+1 &&
				int(st.Packs.Kept) == len(remote.uploaded())
		})
	}
	settled()
	for id, data := range remote.uploaded() {
		if copied, err := os.ReadFile(filepath.Join(dir, "packs", id)); err != nil || !bytes.Equal(copied, data) {
			t.Fatalf("kept copy of %s differs from the upload: %v", id, err)
		}
	}

	// Concurrent misses on a pack the node lacks are served from the remote's
	// range, and fetch the pack once.
	var on atomic.Bool
	on.Store(true)
	var reach atomic.Pointer[Client]
	reach.Store(client)
	reads := NewReads(time.Second, &on, nil, &reach)
	missed := seedIDs[0]
	var readers sync.WaitGroup
	for i := range 8 {
		readers.Go(func() {
			off := int64(i * 1000)
			if data, ok := reads.ReadCached(ctx, missed, off, 100); !ok || !bytes.Equal(data, seeds[missed][off:off+100]) {
				t.Errorf("miss read %d: ok %v", i, ok)
			}
		})
	}
	readers.Wait()
	waitFor(t, func() bool { return node.packs.Stats().Filled == 1 })
	if node.Stats().Backfill != BackfillOff {
		t.Fatal("the backfill runs before it is switched on")
	}

	// The backfill, switched on, fills the rest and stops. Every copy is the
	// remote's pack byte for byte, fetched once.
	node.SetBackfill(true, 1<<30)
	waitFor(t, func() bool { return node.Stats().Backfill == BackfillStopped })
	for id, data := range seeds {
		copied, err := os.ReadFile(filepath.Join(dir, "packs", id))
		if err != nil || !bytes.Equal(copied, data) {
			t.Fatalf("read copy of %s is not the remote's pack: %v", id, err)
		}
	}
	if got := remote.gets.Load(); got != int32(len(seeds)) {
		t.Fatalf("%d fetches of %d packs", got, len(seeds))
	}

	// No fill reached the log, and the uploads hold the pushes alone.
	settled()
	checkUploads := func() {
		t.Helper()
		for id, data := range remote.uploaded() {
			if _, ok := seeds[id]; ok {
				t.Fatalf("uploaded a filled pack %s", id)
			}
			index, err := packfile.ReadIndex(bytes.NewReader(data), uint64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range index {
				if !pushed[string(e.GetKey())] {
					t.Fatalf("uploaded a block no push sent: pack %s", id)
				}
			}
		}
	}
	for id := range seeds {
		if node.log.Has(packfile.BlockKey(mustRef(t, seeds[id]).GetHash())) {
			t.Fatalf("filled pack %s reached the log", id)
		}
	}
	checkUploads()

	// A restart with the disk short prunes read copies, oldest first, back to
	// the fills' floor, leaves the log as it was, uploads nothing more, and a
	// backfill switched on stops at once for want of room.
	uploads := len(remote.uploaded())
	copies := node.packs.Stats().Copies
	end()
	stopRun()
	<-ran
	node.Close()
	logBefore := dirBytes(t, filepath.Join(dir, "log"))
	freeBase.Store(0)
	node = open()
	defer node.Close()
	runCtx, stopRun = context.WithCancel(ctx)
	defer stopRun()
	go node.Run(runCtx)
	if st := node.packs.Stats(); st.Copies != copies {
		t.Fatalf("the list holds %d copies after a restart, want %d", st.Copies, copies)
	}
	waitFor(t, func() bool { return node.packs.Stats().PrunedBytes >= 100<<10 })
	if node.packs.holds(missed) {
		t.Fatal("pruning kept the oldest copy")
	}
	if st := node.packs.Stats(); st.Copies != copies-int64(st.Pruned) || int64(st.Pruned) >= copies { //nolint:gosec // a count
		t.Fatalf("pruned %d of %d copies, want only enough for the floor", st.Pruned, copies)
	}
	if got := dirBytes(t, filepath.Join(dir, "log")); got != logBefore {
		t.Fatalf("pruning changed the log: %d bytes, was %d", got, logBefore)
	}
	freeBase.Store(100<<10 - 1 - int64(node.packs.Stats().PrunedBytes)) //nolint:gosec // a count
	node.SetBackfill(true, 1<<30)
	waitFor(t, func() bool { return node.Stats().Backfill == BackfillStopped })
	if node.packs.holds(missed) {
		t.Fatal("a backfill filled with no room")
	}
	if got := len(remote.uploaded()); got != uploads {
		t.Fatalf("%d uploads after the restart, was %d", got, uploads)
	}
	checkUploads()
}

// TestFillIntoLogMutantFails runs TestNodeFillsNeverUpload against a node that
// appends each pack it fills to its pack log, which then repacks and uploads
// it. The run-through must fail on it.
func TestFillIntoLogMutantFails(t *testing.T) {
	// The mutant builds the package again, so it runs only in full test runs.
	if testing.Short() || os.Getenv(fillMutantEnv) != "" {
		t.Skip("mutant runs need a full test run")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}

	// Route the read copies' source through the log, as a node that held its
	// fills in the log would.
	src, err := os.ReadFile("node.go")
	if err != nil {
		t.Fatal(err)
	}
	const from = "NewPackFiles(ctx, le, filepath.Join(conf.Dir, \"packs\"), conf.Source,"
	const to = "NewPackFiles(ctx, le, filepath.Join(conf.Dir, \"packs\"), fillIntoLog{Source: conf.Source, log: log},"
	if strings.Count(string(src), from) != 1 {
		t.Fatal("the mutant's text no longer matches node.go")
	}
	tmp := t.TempDir()
	mutated := filepath.Join(tmp, "node.go")
	if err := os.WriteFile(mutated, []byte(strings.Replace(string(src), from, to, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	added := filepath.Join(tmp, "fill_into_log.go")
	if err := os.WriteFile(added, []byte(fillIntoLogMutant), 0o600); err != nil {
		t.Fatal(err)
	}

	// Run the run-through against it.
	here, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	overlay := []byte(`{"Replace":{` +
		strconv.Quote(filepath.Join(here, "node.go")) + `:` + strconv.Quote(mutated) + `,` +
		strconv.Quote(filepath.Join(here, "fill_into_log.go")) + `:` + strconv.Quote(added) + `}}`)
	overlayPath := filepath.Join(tmp, "overlay.json")
	if err := os.WriteFile(overlayPath, overlay, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), goBin, "test", "-overlay", overlayPath, "-run", "^TestNodeFillsNeverUpload$", "-count=1", "-timeout=120s", ".")
	cmd.Env = append(os.Environ(), fillMutantEnv+"=1")
	out, err := cmd.CombinedOutput()

	// It must fail on the upload check.
	if err == nil {
		t.Fatalf("the mutant survived:\n%s", out)
	}
	if !strings.Contains(string(out), "uploaded a block no push sent") && !strings.Contains(string(out), "reached the log") {
		t.Fatalf("the mutant failed for another reason:\n%s", out)
	}
}

// fillIntoLogMutant is the mutant's added file: a source that appends every
// pack the node fills to its pack log.
const fillIntoLogMutant = `package block_store_nodepush

import (
	"bytes"
	"context"
	"io"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

type fillIntoLog struct {
	Source
	log *packahead.Log
}

func (f fillIntoLog) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	body, err := f.Source.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return nil, err
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		return nil, err
	}
	if err := f.log.Append(ctx, []packahead.Block{{Key: packfile.BlockKey(ref.GetHash()), Value: data}}); err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
`

// mustRef builds the reference of data.
func mustRef(t *testing.T, data []byte) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// dirBytes sums the sizes of the files in dir.
func dirBytes(t *testing.T, dir string) int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
	}
	return total
}

// TestNodeReadsAreChecked reads a local store's blocks through a real node: a
// miss is the node's range read of the bucket, the next read is its copy, a
// copy that serves wrong bytes fails the store's hash check and the bucket
// answers, and a node that is away costs nothing.
func TestNodeReadsAreChecked(t *testing.T) {
	// A local store on a bucket, reading through a node that holds nothing.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	_, bucket := s3test.New(t)
	var on atomic.Bool
	on.Store(true)
	var reach atomic.Pointer[Client]
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	store := block_store_s3.NewPackStore(le, bucket, s3test.BucketName, "p/", &block_store_s3.PackStoreOptions{
		KeepPacks: true, Index: kv, SoleWriter: true, ExactReads: true, ReadCache: NewReads(time.Second, &on, nil, &reach),
	})
	t.Cleanup(store.Close)
	dir := t.TempDir()
	packs, err := NewPackFiles(ctx, le, dir+"/packs", bucket, s3test.BucketName, "p/packs", PackFilesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = packs.Close() })
	server, err := NewServer(nopHolder{}, dir+"/reports")
	if err != nil {
		t.Fatal(err)
	}
	server.SetPacks(packs)
	client := NewClient(le, &slowApplier{})
	end := connect(ctx, t, client, server)
	reach.Store(client)

	// The store writes a pack of five blocks to the bucket.
	var batch []*block.PutBatchEntry
	for i := range 5 {
		e := entry(t, strings.Repeat("checked block ", 20)+string(rune('a'+i)))
		e.RefsKnown = true
		batch = append(batch, e)
	}
	if _, err := store.PutBlockBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	read := func(i int) {
		t.Helper()
		data, found, err := store.GetBlock(ctx, batch[i].Ref)
		if err != nil || !found || !bytes.Equal(data, batch[i].Data) {
			t.Fatalf("read %d = %q, %v, %v", i, data, found, err)
		}
	}

	// A miss is served by the node's range read of the bucket, once the
	// store's index rows hold the blocks' extents, which a background drain
	// writes after the pack.
	for {
		read(0)
		if got := store.ReadCounts(); got.Cached+got.Bucket != 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := store.ReadCounts(); got.Cached != 1 || got.Bucket != 0 {
		t.Fatalf("counts after a miss: %+v", got)
	}
	if got := packs.Stats(); got.RemoteReads != 1 {
		t.Fatalf("node counts after a miss: %+v", got)
	}

	// The pack fills, and the next read is the node's copy.
	fillCtx, stopFill := context.WithCancel(ctx)
	defer stopFill()
	go packs.Run(fillCtx)
	waitFor(t, func() bool { return packs.Stats().Filled == 1 })
	read(1)
	if got := packs.Stats(); got.CopyReads != 1 {
		t.Fatalf("node counts after a copy read: %+v", got)
	}

	// A copy with wrong bytes fails the hash, and the bucket serves the read.
	names, err := os.ReadDir(dir + "/packs")
	if err != nil || len(names) != 1 {
		t.Fatalf("read copies %v %v", names, err)
	}
	copyPath := filepath.Join(dir, "packs", names[0].Name())
	info, err := os.Stat(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyPath, make([]byte, info.Size()), 0o600); err != nil {
		t.Fatal(err)
	}
	read(2)
	if got := store.ReadCounts(); got.Cached != 2 || got.CacheRejected != 1 || got.Bucket != 1 {
		t.Fatalf("counts after wrong bytes: %+v", got)
	}

	// A node that is away leaves the read to the bucket.
	end()
	read(3)
	if got := store.ReadCounts(); got.Bucket != 2 || got.CacheRejected != 1 {
		t.Fatalf("counts with the node away: %+v", got)
	}
}
