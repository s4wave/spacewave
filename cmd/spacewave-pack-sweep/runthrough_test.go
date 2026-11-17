package main

import (
	"bytes"
	"cmp"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_s3_overlay "github.com/s4wave/spacewave/db/block/store/s3/overlay"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// runSteps are the steps of a cycle after new-cycle, in order.
var runSteps = []string{"snapshot", "walk", "judge", "compact", "quarantine", "recheck", "trash", "remove", "restore"}

// TestCommandRunThrough runs whole cycles, compaction included, while a writer
// keeps putting blocks and moving its root, with the pack index in a file of
// each volume type, on the store and as a rehearsal. Cycle k restarts the
// backend once, before its step k, closing its log cleanly with the writer
// paused, so the restarted log resumes the epoch and the cycle goes on with no
// new walk; and it fails the command's k-th object call in each step that
// reaches the store, then runs the step again. After every step, every block
// the root reaches reads, and no cycle lifts a pack or meets a gap. With the
// writer stopped, a clean cycle leaves no dead or low pack.
func TestCommandRunThrough(t *testing.T) {
	commandRuns(t, func(t *testing.T, kind string, rehearsal bool) {
		testCommandRunThrough(t, kind, rehearsal, packsweep.TrashMove)
	})
}

// TestCommandRunThroughHide is TestCommandRunThrough with the trash hiding
// the packs under delete markers, on the store, and checks that no step
// deleted a content version of the store's objects.
func TestCommandRunThroughHide(t *testing.T) {
	hideRuns(t, func(t *testing.T, kind string) {
		testCommandRunThrough(t, kind, false, packsweep.TrashHide)
	})
}

// testCommandRunThrough runs the cycles with the pack index in a file of the
// volume type kind, as a rehearsal when rehearsal is set, trashing by method.
func testCommandRunThrough(t *testing.T, kind string, rehearsal bool, method packsweep.TrashMethod) {
	r, faults, client, stopWriter := newRunThrough(t, kind, rehearsal)
	defer r.stop()
	defer stopWriter()
	r.args.TrashMethod = method.String()

	// Run a cycle restarting the backend before each step, then a clean one.
	var removed, compacted, lifted, gaps, hidden uint
	for k := 0; k <= len(runSteps); k++ {
		res := r.cycle(k, faults)
		removed += res.removed
		compacted += res.compacted
		lifted += res.lifted
		gaps += res.gaps
		hidden += res.hidden
	}
	if gaps != 0 {
		t.Fatalf("clean restarts sent the cycles back to a new walk %d times", gaps)
	}

	// No read or write reached a quarantined pack, since the writer reads none
	// and links only new leaves.
	stopWriter()
	r.checkReads("the end")
	if removed == 0 || compacted == 0 || lifted != 0 {
		t.Fatalf("the cycles removed %d packs, compacted %d and lifted %d", removed, compacted, lifted)
	}
	if hide := method == packsweep.TrashHide; hide != (hidden != 0) {
		t.Fatalf("the cycles hid %d packs by method %s", hidden, method)
	}
	if method == packsweep.TrashHide {
		// The compaction drops its own unfinished packs, which no hide
		// covered; a version of a key a hide covered must outlive the run.
		deleted := r.bucket.HiddenDeleted("world/")
		t.Logf("hide: removed %d packs, hid %d; %d content versions deleted in all, %d from hidden keys", removed, hidden, r.bucket.ContentDeleted("world/"), len(deleted))
		if len(deleted) != 0 {
			t.Fatalf("the hiding cycles deleted content versions of hidden keys: %v", deleted)
		}
	}

	// With the writer stopped, a clean cycle leaves no dead or low pack, as a
	// judgment after it shows, and the store is unchanged in a rehearsal.
	r.cycle(-1, faults)
	r.mustRun("new-cycle")
	r.readRoots()
	for _, step := range runSteps[:3] {
		r.mustRun(step)
	}
	r.checkReads("the last judgment")
	report, err := os.ReadFile(r.args.Report)
	if err != nil {
		t.Fatal(err)
	}
	for line := range bytes.Lines(bytes.TrimSpace(report)) {
		if v := fastjson.MustParseBytes(line).GetStringBytes("verdict"); string(v) == "dead" || string(v) == "low" {
			t.Fatalf("after a clean cycle the store holds a pack judged %s: %s", v, line)
		}
	}
	if rehearsal {
		if after := readObjects(t, client); len(after) != 0 {
			t.Fatalf("the rehearsal wrote %d objects to the empty store", len(after))
		}
	}
}

// TestCommandRestartWindow restarts the backend after the recheck passed the
// cycle's dead packs, and before the restart, once the log closed, the
// stopping backend takes a write whose root links a block the walk found
// dead. The restarted log must not resume the epoch that missed the write:
// the trash meets a gap, the cycle walks again, and the linked block reads
// after every later step.
func TestCommandRestartWindow(t *testing.T) {
	commandRuns(t, func(t *testing.T, kind string, rehearsal bool) {
		// Restart with the write once, before the trash of the first cycle.
		r, faults, _, stopWriter := newRunThrough(t, kind, rehearsal)
		defer r.stop()
		defer stopWriter()
		trashAt := slices.Index(runSteps, "trash")
		var linked *block.BlockRef
		r.before = func(i int) {
			r.mtx.Lock()
			take := i == trashAt && linked == nil && len(r.prey) != 0
			if take {
				linked = r.prey[0]
				r.prey = r.prey[1:]
			}
			r.mtx.Unlock()
			if take {
				r.restartWithWrite(linked)
			}
		}

		// The cycle meets the gap and still keeps the linked block.
		res := r.cycle(-1, faults)
		r.before = nil
		stopWriter()
		r.checkReads("the cycle")
		if linked == nil || res.gaps == 0 {
			t.Fatalf("linked %v, gaps %d: want a link and a gap", linked != nil, res.gaps)
		}
	})
}

// newRunThrough serves an empty store, through an overlay for a rehearsal,
// starts a backend over it with the pack index in a file of the volume type
// kind, seeds it, starts the writer, and configures the command. The caller
// stops the writer, then the backend.
func newRunThrough(t *testing.T, kind string, rehearsal bool) (*runThrough, *faultObjects, block_store_s3.ObjectStore, func()) {
	// Serve the store, through an overlay for a rehearsal, and fail a chosen
	// object call of the command's.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	dir := t.TempDir()
	bucket, client := s3test.New(t)
	var objects block_store_s3.ObjectMover = client
	if rehearsal {
		ov, err := block_store_s3_overlay.New(client, s3test.BucketName, filepath.Join(dir, "overlay"))
		if err != nil {
			t.Fatal(err)
		}
		objects = ov
	}
	faults := &faultObjects{ObjectMover: objects}

	// Start the backend and the writer, and seed the store.
	r := &runThrough{t: t, ctx: ctx, kind: kind, index: filepath.Join(dir, "index.db"), objects: objects, bucket: bucket, dir: dir}
	r.start()
	for range 12 {
		r.write()
	}
	stopWriter := r.runWriter()

	// Configure the command.
	r.args = &Args{
		Scratch:      filepath.Join(dir, "scratch"),
		StoreID:      commandStoreID,
		Prefix:       "world/",
		Reads:        4,
		Batch:        64,
		MinAge:       time.Nanosecond,
		LowPercent:   packsweep.DefaultLowLivePercent,
		Wait:         time.Nanosecond,
		Report:       filepath.Join(dir, "report.jsonl"),
		Holes:        filepath.Join(dir, "holes.txt"),
		MaxPackBytes: 1 << 20,
		AdminURL:     r.backend.url,
		dial: func() (block_store_s3.ObjectStore, string, func(), error) {
			return faults, s3test.BucketName, func() {}, nil
		},
		out: &r.out,
	}
	return r, faults, client, stopWriter
}

// runThrough is the state of a run-through: the backend it restarts, the
// writer, and the command.
type runThrough struct {
	t   *testing.T
	ctx context.Context
	// kind and index are the pack index file's volume type and path.
	kind, index string
	// objects is the store the backend reads, and bucket the bucket
	// under it.
	objects block_store_s3.ObjectStore
	bucket  *s3test.Bucket
	// dir holds the test's files.
	dir string

	// mtx pauses the writer: it holds mtx for each write and root change, and
	// a restart or a read of the roots holds it too.
	mtx sync.Mutex
	// store, indexFile, closeIndex and backend are the running backend.
	store      *block_store_s3.PackStore
	indexFile  kvtx.Store
	closeIndex func() error
	backend    *commandBackend
	// root is the writer's root, and live the leaves it links.
	root *block.BlockRef
	live []*block.BlockRef
	// writes counts the writer's rounds, copies the index copies, and cycles
	// the cycles begun.
	writes, copies, cycles int
	// data holds each leaf's data by its ref.
	data map[string][]byte
	// linked are the blocks an adversary linked, which the root links and
	// the reads check, and late those it linked too late for the sweep to
	// see, which the root links but no read checks until the cycle's end.
	linked, late []*block.BlockRef
	// dropped are the leaves the root dropped since the cycle's roots were
	// read, and prey those it dropped before them, dead to this cycle's walk.
	dropped, prey []*block.BlockRef
	// before is called before each step's first run, with its index.
	before func(i int)

	// args and out are the command and its output.
	args *Args
	out  bytes.Buffer
}

// start opens the backend over the index file, its writes logged.
func (r *runThrough) start() {
	r.store, r.indexFile, r.closeIndex = openIndexFile(r.ctx, r.t, r.kind, r.index, r.objects)
	r.backend = serveAdmin(r.t, r.store, r.dir)
	r.backend.marking.SetObserver(r.backend.wlog)
	if r.args != nil {
		r.args.AdminURL = r.backend.url
	}
}

// stop closes the backend, as a crash would leave its files.
func (r *runThrough) stop() {
	r.backend.stop()
	r.store.Close()
	if err := r.closeIndex(); err != nil {
		r.t.Error(err)
	}
}

// restart stops the backend and starts it again, with the writer paused.
func (r *runThrough) restart() {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.stop()
	r.start()
}

// restartWithWrite stops the backend, closing its log, then has the stopped
// backend's store take a write whose root links ref, as a write landing after
// the log closed in a process that is shutting down, and starts the backend
// again.
func (r *runThrough) restartWithWrite(ref *block.BlockRef) {
	// Close the log, with the writer paused.
	r.mtx.Lock()
	r.backend.stop()
	r.linked = append(r.linked, ref)
	r.mtx.Unlock()

	// Write through the old store, then start the backend again.
	r.write()
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.stop()
	r.start()
}

// write puts a round's leaf and a new root linking it and the last four
// leaves, then moves the root to it. The leaf a round drops is dead, and the
// pack holding it and the root before is dead once the root moves again.
func (r *runThrough) write() {
	// Build the leaf and the root, the root's data large so a pack whose
	// leaf lives and root died is low.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	n := strconv.Itoa(r.writes)
	r.writes++
	leafData := []byte("leaf " + n)
	leaf, err := block.BuildBlockRef(leafData, nil)
	if err != nil {
		r.t.Error(err)
		return
	}
	if r.data == nil {
		r.data = make(map[string][]byte)
	}
	r.data[leaf.MarshalString()] = leafData
	r.live = append(r.live, leaf)
	if len(r.live) > 5 {
		r.dropped = append(r.dropped, r.live[0])
		r.live = r.live[1:]
	}

	// Put them in one batch, which uploads one pack, and move the root.
	rootData := []byte("root " + n + " " + strings.Repeat("x", 2000))
	root, err := block.BuildBlockRef(rootData, nil)
	if err != nil {
		r.t.Error(err)
		return
	}
	_, err = r.backend.marking.PutBlockBatch(r.ctx, []*block.PutBatchEntry{
		{Ref: leaf, Data: leafData, RefsKnown: true},
		{Ref: root, Data: rootData, Refs: slices.Concat(r.live, r.linked, r.late), RefsKnown: true},
	})
	if err != nil {
		r.t.Error(err)
		return
	}
	r.root = root
	r.backend.wlog.Root(root.GetHash())
}

// runWriter writes a round every few milliseconds until the returned func is
// called.
func (r *runThrough) runWriter() func() {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			case <-time.After(5 * time.Millisecond):
				r.write()
			}
		}
	}()
	return sync.OnceFunc(func() {
		close(done)
		wg.Wait()
	})
}

// checkReads reads the root and every leaf and linked block it links but
// the late, with the writer paused.
func (r *runThrough) checkReads(step string) {
	r.t.Helper()
	r.mtx.Lock()
	defer r.mtx.Unlock()
	for _, ref := range slices.Concat([]*block.BlockRef{r.root}, r.live, r.linked) {
		if _, found, err := r.store.GetBlock(r.ctx, ref); err != nil || !found {
			r.t.Fatalf("after the %s, read of live block %s = %v, %v", step, ref.MarshalString(), found, err)
		}
	}
}

// readRoots sets the cycle's root and log position, read at one instant
// before its snapshot, and names the pack index for --index: the backend's
// own file, which the snapshot reads beside it, or for bbolt, whose writer
// holds its lock, a copy taken there; a step run again keeps them.
func (r *runThrough) readRoots() {
	// Read the roots and the position with the writer paused.
	r.t.Helper()
	r.mtx.Lock()
	defer r.mtx.Unlock()
	pos, ok := r.backend.wlog.Position()
	if !ok {
		r.t.Fatal("write log is off")
	}
	r.args.Roots = *cli.NewStringSlice(r.root.MarshalString())
	r.args.LogPosition = pos.String()

	// Prey are the blocks dead to this cycle's walk alone: one left from an
	// earlier cycle may be in a pack that cycle removed.
	r.prey = r.dropped
	r.dropped = nil

	// Name the backend's SQLite file, which opens read-only beside its writer.
	// Copy the others in one read to a new file: bbolt's writer holds its
	// lock, and s4db refuses a second open of a file in one process, as the
	// backend's is here.
	if r.kind == packcopy.IndexSQLite {
		r.args.Index = r.index
		return
	}
	r.copies++
	r.args.Index = filepath.Join(r.dir, "index-copy-"+strconv.Itoa(r.copies)+".db")
	if err := copyIndex(r.ctx, r.indexFile, r.kind, r.args.Index); err != nil {
		r.t.Fatal(err)
	}
}

// copyIndex copies every key of src, in one read, to a new file of the
// volume type kind at path.
func copyIndex(ctx context.Context, src kvtx.Store, kind, path string) error {
	// Open the read and the copy.
	rtx, err := src.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer rtx.Discard()
	dst, closeDst, err := packcopy.OpenIndexFile(ctx, path, &packcopy.IndexFileOptions{
		Kind:   kind,
		Table:  defaultIndexTable,
		SQLite: store_kvtx_sqlite.Pragmas{WithoutRowID: true},
	})
	if err != nil {
		return err
	}

	// Write every key in one transaction, and close the copy.
	wtx, err := dst.NewTransaction(ctx, true)
	if err == nil {
		err = rtx.ScanPrefix(ctx, nil, func(key, value []byte) error {
			return wtx.Set(ctx, bytes.Clone(key), bytes.Clone(value))
		})
		if err == nil {
			err = wtx.Commit(ctx)
		}
		wtx.Discard()
	}
	return cmp.Or(err, closeDst())
}

// run runs a step of the command and parses its output.
func (r *runThrough) run(step string) (*fastjson.Value, error) {
	// Run the step's action.
	actions := map[string]func(*cli.Context) error{
		"new-cycle": r.args.runNewCycle, "snapshot": r.args.runSnapshot, "walk": r.args.runWalk,
		"judge": r.args.runJudge, "compact": r.args.runCompact, "quarantine": r.args.runQuarantine,
		"recheck": r.args.runRecheck, "trash": r.args.runTrash, "remove": r.args.runRemove,
		"restore": r.args.runRestore,
	}
	r.out.Reset()
	if err := actions[step](nil); err != nil {
		return nil, err
	}

	// Parse the output, the judgment's last line when it prints several.
	out := bytes.TrimSpace(r.out.Bytes())
	if i := bytes.LastIndexByte(out, '\n'); i >= 0 {
		out = out[i+1:]
	}
	if len(out) == 0 {
		return nil, nil
	}
	return fastjson.ParseBytes(out)
}

// cycleResult counts what a cycle did.
type cycleResult struct {
	removed, compacted, lifted, gaps, hidden uint
}

// cycle runs one cycle, restarting the backend before step restartAt and
// failing the command's restartAt-th object call once in each step. A step a
// gap blocks sends the cycle back to its walk.
func (r *runThrough) cycle(restartAt int, faults *faultObjects) cycleResult {
	r.t.Helper()
	var res cycleResult
	restarted := false
	r.cycles++
	r.mustRun("new-cycle")
	r.readRoots()
	for i := 0; i < len(runSteps); i++ {
		// Restart the backend once, before the chosen step, and fail one
		// object call of the step's first run.
		step := runSteps[i]
		if i == restartAt && !restarted {
			r.restart()
			restarted = true
		}
		if r.before != nil {
			r.before(i)
		}
		faults.failAt(restartAt + 1)
		out, err := r.run(step)
		faults.failAt(0)
		if err != nil {
			if !errors.Is(err, errFault) {
				r.t.Fatalf("cycle %d: %s: %v", restartAt, step, err)
			}
			out = r.mustRun(step)
		}
		r.checkReads(step)

		// A gap sends the cycle back to a new walk.
		if blocked := string(out.GetStringBytes("blocked")); blocked == packsweep.BlockLogGap {
			res.gaps++
			r.mustRun("new-cycle")
			r.readRoots()
			i = -1
			continue
		} else if blocked != "" {
			holes, _ := os.ReadFile(r.args.Holes)
			r.t.Fatalf("cycle %d: %s blocked: %s\n%s", restartAt, step, out, holes)
		}

		// Recheck until every quarantined pack passed or was lifted.
		switch step {
		case "recheck":
			res.lifted += out.GetUint("lifted") + out.GetUint("live")
			for tries := 0; len(out.GetObject("waiting").String()) > 2 && out.GetUint("passed") == 0; tries++ {
				if tries == 200 {
					r.t.Fatalf("cycle %d: recheck still waits: %s", restartAt, out)
				}
				time.Sleep(5 * time.Millisecond)
				out = r.mustRun("recheck")
				res.lifted += out.GetUint("lifted") + out.GetUint("live")
			}
		case "compact":
			res.compacted += out.GetUint("victims")
		case "trash":
			res.hidden += out.GetUint("hidden")
		case "remove":
			res.removed += out.GetUint("removed")
		}
	}
	return res
}

// mustRun runs a step that must succeed.
func (r *runThrough) mustRun(step string) *fastjson.Value {
	r.t.Helper()
	out, err := r.run(step)
	if err != nil {
		r.t.Fatalf("%s: %v", step, err)
	}
	return out
}

// errFault is the error of an injected failure.
var errFault = errors.New("injected object call failure")

// faultObjects fails one chosen object call, counting from when it was armed.
type faultObjects struct {
	block_store_s3.ObjectMover
	// at is the call to fail, zero for none; calls counts the calls since.
	at, calls atomic.Int64
}

// failAt arms the n-th call from now to fail, zero for none.
func (f *faultObjects) failAt(n int) {
	f.calls.Store(0)
	f.at.Store(int64(n))
}

// fault reports whether this call fails.
func (f *faultObjects) fault() error {
	if at := f.at.Load(); at > 0 && f.calls.Add(1) == at {
		return errFault
	}
	return nil
}

// PutObject fails or writes.
func (f *faultObjects) PutObject(ctx context.Context, bucket, key string, data []byte, contentType string) error {
	if err := f.fault(); err != nil {
		return err
	}
	return f.ObjectMover.PutObject(ctx, bucket, key, data, contentType)
}

// GetObject fails or reads.
func (f *faultObjects) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	if err := f.fault(); err != nil {
		return nil, err
	}
	return f.ObjectMover.GetObject(ctx, bucket, key)
}

// GetObjectRange fails or reads.
func (f *faultObjects) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	if err := f.fault(); err != nil {
		return nil, err
	}
	return f.ObjectMover.GetObjectRange(ctx, bucket, key, off, length)
}

// ListObjects fails or lists.
func (f *faultObjects) ListObjects(ctx context.Context, bucket, prefix string, fn func(key string, size int64) error) error {
	if err := f.fault(); err != nil {
		return err
	}
	return f.ObjectMover.ListObjects(ctx, bucket, prefix, fn)
}

// DeleteObject fails or deletes.
func (f *faultObjects) DeleteObject(ctx context.Context, bucket, key string) error {
	if err := f.fault(); err != nil {
		return err
	}
	return f.ObjectMover.DeleteObject(ctx, bucket, key)
}

// MoveObject fails or moves.
func (f *faultObjects) MoveObject(ctx context.Context, bucket, from, to string) error {
	if err := f.fault(); err != nil {
		return err
	}
	return f.ObjectMover.MoveObject(ctx, bucket, from, to)
}

// hider returns the store as an ObjectHider, or an error when it is none, as
// an overlay is not.
func (f *faultObjects) hider() (block_store_s3.ObjectHider, error) {
	if err := f.fault(); err != nil {
		return nil, err
	}
	hider, ok := f.ObjectMover.(block_store_s3.ObjectHider)
	if !ok {
		return nil, errors.New("the store cannot hide its objects")
	}
	return hider, nil
}

// HideObjectVersion fails or hides.
func (f *faultObjects) HideObjectVersion(ctx context.Context, bucket, key string) (block_store_s3.HiddenObject, error) {
	hider, err := f.hider()
	if err != nil {
		return block_store_s3.HiddenObject{}, err
	}
	return hider.HideObjectVersion(ctx, bucket, key)
}

// UnhideObjectVersion fails or unhides.
func (f *faultObjects) UnhideObjectVersion(ctx context.Context, bucket, key, version string) error {
	hider, err := f.hider()
	if err != nil {
		return err
	}
	return hider.UnhideObjectVersion(ctx, bucket, key, version)
}

// HiddenObjectVersion fails or finds the hidden version.
func (f *faultObjects) HiddenObjectVersion(ctx context.Context, bucket, key string) (string, error) {
	hider, err := f.hider()
	if err != nil {
		return "", err
	}
	return hider.HiddenObjectVersion(ctx, bucket, key)
}

// GetObjectVersionRange fails or reads the version.
func (f *faultObjects) GetObjectVersionRange(ctx context.Context, bucket, key, version string, off int64, length int) ([]byte, error) {
	hider, err := f.hider()
	if err != nil {
		return nil, err
	}
	return hider.GetObjectVersionRange(ctx, bucket, key, version, off, length)
}
