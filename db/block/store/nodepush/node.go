package block_store_nodepush

import (
	"context"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/sirupsen/logrus"
)

// uploadRetry is how long a failed drain waits before it runs again.
const uploadRetry = 5 * time.Second

// pruneCheck is how often the node compares its free disk with the prune mark.
// The pack log grows at most a few MB a second, so a check this often keeps
// far ahead of any mark measured in GiB.
const pruneCheck = 30 * time.Second

// defaultBackfillBytesPerSecond paces a backfill whose rate is unset.
const defaultBackfillBytesPerSecond = 8 << 20

// NodeConfig configures a Node.
type NodeConfig struct {
	// Dir holds the pack log in log/, its reports in reports/, and the read
	// copies in packs/ with their list in packs.db. The log and the read
	// copies never share a directory.
	Dir string
	// Linger rotates a pack once its first batch is this old.
	Linger time.Duration
	// KeepBytes is the size of uploaded packs the log keeps, which bounds the
	// memory its table takes.
	KeepBytes int64
	// Remote is the pack store the log's packs upload to.
	Remote packahead.Remote
	// Source reads the remote's packs for the read copies.
	Source Source
	// Bucket is the bucket of the packs in Source.
	Bucket string
	// Prefix is the key prefix of the packs in Source, before the pack ID.
	Prefix string
	// List walks the remote's packs with their sizes for the backfill. Nil
	// leaves the backfill unavailable.
	List func(ctx context.Context, fn func(id string, size int64) error) error
	// RemoteReads bounds the range reads of the remote for packs the node
	// lacks, as PackFilesOptions describes.
	RemoteReads int
	// MinFreeBytes is the free disk below which the node keeps and fills no
	// read copies, so they never take the disk the pushes need.
	MinFreeBytes uint64
	// PruneFreeBytes is the free disk below which the node prunes read copies,
	// oldest first, until MinFreeBytes is free. It sits below MinFreeBytes, so
	// fills stop well before pruning starts and the two never trade one pack
	// for another. Zero never prunes.
	PruneFreeBytes uint64
	// FreeBytes reports the free disk of Dir, false when unknown. Nil never
	// knows, so read copies always have room and nothing is pruned.
	FreeBytes func() (uint64, bool)
}

// NodeStats is what a Node holds and has done.
type NodeStats struct {
	// FreeBytes is the free disk, and FreeKnown whether it could be read.
	FreeBytes uint64
	// FreeKnown reports whether FreeBytes was read.
	FreeKnown bool
	// Log counts the pack log's packs and bytes.
	Log packahead.Stats
	// Upload counts the log's packs still to upload.
	Upload packahead.DrainStatus
	// ReportsPending counts the uploaded packs the local store has not applied.
	ReportsPending int
	// Packs counts the read copies and their reads, fills and prunes.
	Packs PackFilesStats
	// Backfill is the backfill's state.
	Backfill BackfillState
}

// BackfillState is the state of a Node's backfill.
type BackfillState int

const (
	// BackfillOff is a backfill switched off, the default.
	BackfillOff BackfillState = iota
	// BackfillRunning is a backfill walking the remote's packs.
	BackfillRunning
	// BackfillStopped is a backfill switched on that finished its walk, failed
	// or stopped for want of disk; switching it off and on walks again.
	BackfillStopped
)

// Node is the storage node: it holds a local store's pushes in a pack log,
// uploads the log's packs to the remote and reports them back, and serves the
// local store's reads from read copies of the remote's packs.
//
// The pack log and the read copies are kept apart by construction: the
// server appends pushes to the log, the drain uploads only the log's packs,
// and the read copies, filled from the remote or kept from an upload, live in
// their own directory with no handle to the log. So a pack that came from the
// remote is never uploaded or repacked again.
type Node struct {
	// le logs the node's background work.
	le *logrus.Entry
	// conf is the configuration.
	conf NodeConfig
	// log holds the pushes until they upload.
	log *packahead.Log
	// server is the connection's end, in front of log and packs.
	server *Server
	// journal keeps the local store's commit records.
	journal *Journal
	// packs serves the reads from read copies.
	packs *PackFiles
	// drain uploads the log's finalized packs.
	drain *packahead.Drain
	// backfillWake tells the backfill routine its switch changed.
	backfillWake chan struct{}
	// backfillRate is the backfill's pace in bytes a second.
	backfillRate atomic.Int64
	// uploadRate paces the upload in bytes a second, zero for no pace.
	uploadRate atomic.Int64
	// uploadOff holds the upload before its next pack, and the reports.
	uploadOff atomic.Bool
	// uploadWake tells a held upload its switch changed.
	uploadWake chan struct{}

	// mtx guards backfillOn and backfill.
	mtx sync.Mutex
	// backfillOn is the backfill's switch.
	backfillOn bool
	// backfill is the backfill's state.
	backfill BackfillState
}

// OpenNode opens the node's pack log, reports and read copies under
// conf.Dir. Close releases them after Run returns.
func OpenNode(ctx context.Context, le *logrus.Entry, conf NodeConfig) (*Node, error) {
	// Open the pack log and the server in front of it.
	log, err := packahead.Open(ctx, filepath.Join(conf.Dir, "log"), packahead.Options{Linger: conf.Linger})
	if err != nil {
		return nil, errors.Wrap(err, "open pack log")
	}
	server, err := NewServer(log, filepath.Join(conf.Dir, "reports"))
	if err != nil {
		log.Close()
		return nil, err
	}

	// Keep the local store's commit records, for its durable mode.
	journal, err := OpenJournal(filepath.Join(conf.Dir, "journal"))
	if err != nil {
		log.Close()
		return nil, errors.Wrap(err, "open journal")
	}
	server.SetJournal(journal)

	// Serve reads from read copies, in their own directory, while the disk
	// has room.
	packs, err := NewPackFiles(ctx, le, filepath.Join(conf.Dir, "packs"), conf.Source, conf.Bucket, conf.Prefix, PackFilesOptions{RemoteReads: conf.RemoteReads})
	if err != nil {
		_ = journal.Close()
		log.Close()
		return nil, err
	}
	n := &Node{
		le:           le,
		conf:         conf,
		log:          log,
		server:       server,
		journal:      journal,
		packs:        packs,
		drain:        nil,
		backfillWake: make(chan struct{}, 1),
		uploadWake:   make(chan struct{}, 1),
	}
	n.drain = packahead.NewDrain(log, packahead.DrainOptions{KeepBytes: conf.KeepBytes, Admit: n.admitUpload})
	n.backfillRate.Store(defaultBackfillBytesPerSecond)
	packs.SetRoom(n.hasRoom)
	server.SetPacks(packs)
	return n, nil
}

// Close closes the read copy list, the journal and the pack log. Call it
// after Run returns.
func (n *Node) Close() {
	// Close each store.
	if err := n.packs.Close(); err != nil {
		n.le.WithError(err).Warn("close the read copy list")
	}
	if err := n.journal.Close(); err != nil {
		n.le.WithError(err).Warn("close the journal")
	}
	n.log.Close()
}

// Run fills read copies, uploads the log's packs, prunes read copies when the
// disk runs short and runs the backfill while it is on, until ctx ends.
func (n *Node) Run(ctx context.Context) {
	// Run each routine, and wait for them all.
	var running sync.WaitGroup
	running.Go(func() { n.packs.Run(ctx) })
	running.Go(func() { n.runDrain(ctx) })
	running.Go(func() { n.runPrune(ctx) })
	running.Go(func() { n.runBackfill(ctx) })
	running.Wait()
}

// Serve runs one connection from the local store until it ends.
func (n *Node) Serve(ctx context.Context, conn net.Conn) error {
	return n.server.Serve(ctx, conn)
}

// SetBackfill switches the backfill and sets its pace, taking effect at once.
// The backfill fills every pack the remote lists that the node lacks into read
// copies, at most bytesPerSecond, after the packs reads missed on, and stops
// below the free disk the read copies leave the pushes. Zero or less keeps the
// default pace.
func (n *Node) SetBackfill(on bool, bytesPerSecond int64) {
	// Set the pace, which the running walk reads before each pack.
	if bytesPerSecond <= 0 {
		bytesPerSecond = defaultBackfillBytesPerSecond
	}
	n.backfillRate.Store(bytesPerSecond)

	// Flip the switch and wake the routine.
	n.mtx.Lock()
	n.backfillOn = on
	n.mtx.Unlock()
	select {
	case n.backfillWake <- struct{}{}:
	default:
	}
}

// SetCopies turns the node's new read copies on or off, at once, as
// PackFiles.SetCopies describes.
func (n *Node) SetCopies(on bool) {
	n.packs.SetCopies(on)
}

// SetUploadRate paces the upload at bytesPerSecond, at once; zero or less
// uploads each pack as soon as it is finalized. The upload sends one pack at
// a time, so the pace bounds its average read rate on the node's disk.
func (n *Node) SetUploadRate(bytesPerSecond int64) {
	n.uploadRate.Store(max(bytesPerSecond, 0))
}

// SetUpload turns the upload and its reports on or off, at once. Off, the
// upload starts no pack and the server sends no report: the pack log keeps
// every pack, nothing is dropped, and the local store keeps the blocks it
// holds until the upload is back on and their reports arrive. A pack being
// uploaded when it goes off finishes.
func (n *Node) SetUpload(on bool) {
	// Flip both switches, then wake what waits on them.
	n.uploadOff.Store(!on)
	n.server.SetReports(on)
	select {
	case n.uploadWake <- struct{}{}:
	default:
	}
}

// admitUpload waits before an upload while the upload is off, then long
// enough that the one before it, lastBytes, ran at the upload's pace.
func (n *Node) admitUpload(ctx context.Context, lastBytes int64) error {
	// Hold while off.
	for n.uploadOff.Load() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-n.uploadWake:
		}
	}

	// No pace, or nothing uploaded yet, waits for nothing.
	rate := n.uploadRate.Load()
	if rate <= 0 || lastBytes <= 0 {
		return nil
	}

	// Wait out the pack's share of the pace.
	timer := time.NewTimer(time.Duration(float64(lastBytes) / float64(rate) * float64(time.Second)))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Stats returns what the node holds and has done.
func (n *Node) Stats() NodeStats {
	// Read the log, the read copies and the disk.
	st := NodeStats{Log: n.log.GetStats(), Packs: n.packs.Stats()}
	st.FreeBytes, st.FreeKnown = n.freeBytes()
	st.Upload, _ = n.drain.Status()

	// Read the reports and the backfill under their locks.
	n.server.mtx.Lock()
	st.ReportsPending = len(n.server.pending)
	n.server.mtx.Unlock()
	n.mtx.Lock()
	st.Backfill = n.backfill
	n.mtx.Unlock()
	return st
}

// runDrain uploads the log's finished packs until ctx ends, retrying a
// failure, and reports each one through the server.
func (n *Node) runDrain(ctx context.Context) {
	remote := NewReportingRemote(n.conf.Remote, n.server)
	for ctx.Err() == nil {
		// Run, and pause after a failure.
		err := n.drain.Run(ctx, remote)
		if ctx.Err() != nil {
			return
		}
		n.le.WithError(err).Warn("upload packs")
		select {
		case <-ctx.Done():
		case <-time.After(uploadRetry):
		}
	}
}

// runPrune prunes read copies while the free disk is under the prune mark,
// until ctx ends.
func (n *Node) runPrune(ctx context.Context) {
	// Pruning is off without a mark.
	if n.conf.PruneFreeBytes == 0 {
		return
	}
	tick := time.NewTicker(pruneCheck)
	defer tick.Stop()
	for {
		// Prune past the mark, back up to the fills' floor.
		if free, ok := n.freeBytes(); ok && free < n.conf.PruneFreeBytes {
			pruned, err := n.packs.Prune(ctx, func() bool {
				free, ok := n.freeBytes()
				return ok && free < n.conf.MinFreeBytes
			})
			n.le.WithError(err).WithField("bytes", pruned).Info("pruned read copies for disk space")
		}

		// Wait for the next check.
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// runBackfill runs the backfill while its switch is on, until ctx ends. Each
// switch on walks the remote's listing once; a walk that ends stays stopped
// until the switch goes off and on again.
func (n *Node) runBackfill(ctx context.Context) {
	for ctx.Err() == nil {
		// Wait for the switch to go on.
		if !n.waitBackfill(ctx, true) {
			return
		}

		// Walk once, until the walk ends or the switch goes off.
		if !n.walkBackfill(ctx) {
			continue
		}

		// Stay stopped until the switch goes off.
		if !n.waitBackfill(ctx, false) {
			return
		}
	}
}

// waitBackfill waits until the backfill's switch is on, or off, and reports
// false once ctx ends. Off marks the backfill off.
func (n *Node) waitBackfill(ctx context.Context, on bool) bool {
	for {
		// Return once the switch is where it is wanted.
		n.mtx.Lock()
		now := n.backfillOn
		if !now {
			n.backfill = BackfillOff
		}
		n.mtx.Unlock()
		if now == on && (!on || n.conf.List != nil) {
			return true
		}

		// Wait for it to change.
		select {
		case <-ctx.Done():
			return false
		case <-n.backfillWake:
		}
	}
}

// walkBackfill walks the remote's packs once, and reports whether the walk
// ended by itself rather than for the switch or ctx.
func (n *Node) walkBackfill(ctx context.Context) bool {
	// Start the walk.
	walkCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	n.mtx.Lock()
	n.backfill = BackfillRunning
	n.mtx.Unlock()
	n.le.WithField("bytes-per-second", n.backfillRate.Load()).Info("backfill started")
	go func() {
		done <- n.packs.Backfill(walkCtx, func(fn func(id string, size int64) error) error {
			return n.conf.List(walkCtx, fn)
		}, n.backfillRate.Load)
	}()

	// Wait for it to end, stopping it when the switch goes off.
	for {
		select {
		case err := <-done:
			n.le.WithError(err).Info("backfill ended")
			n.mtx.Lock()
			n.backfill = BackfillStopped
			n.mtx.Unlock()
			return true
		case <-ctx.Done():
			cancel()
			<-done
			return false
		case <-n.backfillWake:
			n.mtx.Lock()
			on := n.backfillOn
			n.mtx.Unlock()
			if !on {
				cancel()
				n.le.WithError(<-done).Info("backfill switched off")
				return false
			}
		}
	}
}

// hasRoom reports whether the disk keeps MinFreeBytes past a read copy.
// Unknown free space counts as room: a full disk fails only the copy.
func (n *Node) hasRoom() bool {
	free, ok := n.freeBytes()
	return !ok || free >= n.conf.MinFreeBytes
}

// freeBytes reports the free disk of the node's directory.
func (n *Node) freeBytes() (uint64, bool) {
	if n.conf.FreeBytes == nil {
		return 0, false
	}
	return n.conf.FreeBytes()
}
