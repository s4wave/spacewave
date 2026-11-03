package unixfs_checkout

import (
	"context"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// attempt owns one worker's checkout inputs and tree codec.
type attempt struct {
	le    *logrus.Entry
	root  string
	codec Codec
}

// checkout makes directory hold the files of parts, which check approves,
// sharing the worker of every caller in this process that asks for the same
// directory and parts. what names the release in errors and the log, which
// records how long this caller waited.
func (s *attempt) checkout(ctx context.Context, registry *registry, fetch Fetch, what, directory string, parts []*block.BlockRef, check func(staging string) error) error {
	// Name the immutable content before joining its shared worker.
	var key strings.Builder
	key.WriteString(directory)
	for _, root := range parts {
		if root.GetEmpty() {
			return errors.Errorf("%s has no stored files", what)
		}
		key.WriteString("\x00" + root.MarshalString())
	}
	started := time.Now()
	err := registry.run(ctx, key.String(), fetch, func(ctx context.Context, report func(Progress), yield func(context.Context) error) error {
		return s.prepare(ctx, fetch.Remotes, what, directory, parts, check, report, yield)
	})
	s.le.WithError(err).
		WithField("checkout", what).
		WithField("background", fetch.Background).
		WithField("waited", time.Since(started).Round(time.Millisecond).String()).
		Debug("content checkout returned")
	return err
}

// prepare is one worker's attempt at a checkout. It holds directory's lease
// from its first look at the directory until it removed the blocks it
// fetched, so no other writer or eviction, in this process or another, touches
// the directory, its staging or its cache meanwhile. It reuses a directory
// check approves: a release publishes once. It replaces one check rejects,
// such as a checkout an older desktop half deleted or one holding a file the
// PC cannot read. Otherwise it writes the files beside directory, reading the
// blocks the previous checkout beside it shares from its files and those an
// interrupted attempt fetched from the cache beside it, and moves them into
// place at once. A failed or canceled attempt keeps the cache for the next.
// Each attempt logs one line with the time each phase took and the bytes
// each kind of source served while fetching; assembly reads them again.
func (s *attempt) prepare(ctx context.Context, open func(context.Context) ([]block.StoreOps, func()), what, directory string, parts []*block.BlockRef, check func(staging string) error, report func(Progress), yield func(context.Context) error) (err error) {
	// Time each phase, and log the attempt when it ends.
	clock := newPhaseClock()
	var blocks *sources
	var fetched logrus.Fields
	shown := report
	report = func(progress Progress) {
		if progress.Phase != clock.phase {
			clock.enter(progress.Phase)
			if progress.Phase == PhaseAssembling {
				fetched = blocks.fields()
			}
		}
		shown(progress)
	}
	outcome := "reused"
	defer func() {
		// Record progress and transfer bytes when this phase ends.
		le := s.le.WithField("checkout", what).WithFields(clock.fields())
		if fetched == nil && blocks != nil {
			fetched = blocks.fields()
		}
		le = le.WithFields(fetched)
		if err != nil {
			le.WithError(err).Info("content checkout stopped")
			return
		}
		le.WithField("outcome", outcome).Info("content checkout prepared")
	}()

	// Take the destination, waiting for another process's writer.
	report(Progress{Phase: PhaseChecking})
	if err := os.MkdirAll(filepath.Dir(directory), 0o755); err != nil {
		return err
	}
	locks, scope, err := DestinationLease(s.root, directory)
	if err != nil {
		return err
	}
	lease, held, err := locks.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		return err
	}
	if !held {
		report(Progress{Phase: PhaseWaiting})
		if lease, err = locks.WaitAcquireWriteLease(ctx, scope); err != nil {
			return err
		}
		report(Progress{Phase: PhaseChecking})
	}
	defer func() { _ = lease.Release(context.Background()) }()

	// Reuse a complete checkout, and replace an incomplete or unreadable one.
	if _, err := os.Stat(directory); err == nil {
		err := check(directory)
		if err == nil {
			markUsed(directory)
			return nil
		}
		s.le.WithError(err).Warnf("replacing incomplete checkout of %s", what)

	}

	// Write the files beside the destination.
	staging, err := os.MkdirTemp(filepath.Dir(directory), "."+filepath.Base(directory)+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	// Read missing blocks through resumable local storage and caller remotes.
	remotes, release := open(ctx)
	defer release()
	cache := &blockCache{dir: filepath.Join(filepath.Dir(directory), "."+filepath.Base(directory)+".blocks")}
	outcome = "written"
	blocks = newSources(s.le, seed(ctx, s.le, s.codec, directory, parts[0]), cache, remotes)
	if err := s.write(ctx, blocks, parts, staging, report, yield); err != nil {
		return errors.Wrapf(err, "Download %s", what)
	}
	if err := check(staging); err != nil {
		return err
	}

	// Publish the complete tree at once, then drop the fetched blocks, which
	// only this lease holder reads.
	if err := publish(staging, directory); err != nil {
		return err
	}
	markUsed(directory)
	if err := os.RemoveAll(cache.dir); err != nil {
		s.le.WithError(err).Debug("keeping the fetched blocks until the next eviction")
	}
	return nil
}

// write writes the files of every part into staging, reading their blocks
// from blocks, and fails once they stop arriving. yield holds a remote read
// while a foreground checkout needs the network; a held read does not count
// as stalled.
func (s *attempt) write(ctx context.Context, blocks *sources, parts []*block.BlockRef, staging string, progress func(Progress), yield func(context.Context) error) error {
	// Give up once nothing arrives for stallTimeout, so a source that
	// stops answering fails the checkout, which its caller retries, instead
	// of holding it at the same percent.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var held atomic.Int32
	var stall *time.Timer
	stall = time.AfterFunc(math.MaxInt64, func() {
		if held.Load() != 0 {
			stall.Reset(stallTimeout)
			return
		}
		cancel(errStalled)
	})
	stall.Reset(stallTimeout)
	defer stall.Stop()

	// Exclude time yielding to foreground work from the stall deadline.
	blocks.yield = func(ctx context.Context) error {
		held.Add(1)
		defer held.Add(-1)
		return yield(ctx)
	}
	err := s.copyParts(ctx, blocks, parts, staging, func(count Progress) {
		stall.Reset(stallTimeout)
		progress(count)
	})
	if cause := context.Cause(ctx); err != nil && errors.Is(cause, errStalled) {
		return cause
	}
	return err
}

// stallTimeout is how long a checkout waits for its next block or chunk. A
// block is at most a few mebibytes, so only a source that stopped answering
// takes this long.
const stallTimeout = time.Minute

// errStalled ends a checkout whose sources stopped answering.
var errStalled = errors.New("no content arrived for a minute")

// fetchConcurrency is how many blocks a checkout reads at once. Release
// blocks average a few kilobytes, so a distant link spends its time on round
// trips unless many are in flight.
const fetchConcurrency = 32

// copyParts writes the files of every part into staging, reading their blocks
// from blocks. It first fetches every block, calling progress as each
// arrives, then copies the files from the fetched blocks, calling progress
// with the bytes written after each chunk.
func (s *attempt) copyParts(ctx context.Context, blocks block.StoreOps, parts []*block.BlockRef, staging string, progress func(Progress)) error {
	// Open the staging tree.
	tree, err := os.OpenRoot(staging)
	if err != nil {
		return err
	}
	defer tree.Close()

	// Fetch every block in parallel, counting each part's bytes after those
	// of the parts before it, so the copy below, which reads a block at a
	// time, reads them from the seed and the cache.
	count := Progress{Phase: PhaseFetching}
	progress(count)
	for _, root := range parts {
		before := count
		err := s.codec.Prefetch(ctx, s.le, blocks, root, fetchConcurrency, func(total, fetched uint64) {
			count = Progress{Phase: PhaseFetching, Done: before.Done + fetched, Total: before.Total + total}
			progress(count)
		})
		if err != nil {
			return err
		}
	}

	// Copy each part's files, counting the bytes written against the total
	// the fetch found.
	written := Progress{Phase: PhaseAssembling, Total: count.Total}
	progress(written)
	for _, root := range parts {
		err := s.codec.Read(ctx, s.le, blocks, root, func(part fs.FS) error {
			return fs.WalkDir(part, ".", func(name string, entry fs.DirEntry, err error) error {
				if err != nil || name == "." {
					return err
				}
				if entry.IsDir() {
					return tree.MkdirAll(name, 0o755)
				}
				return copyFile(part, tree, name, func(n uint64) {
					written.Done += n
					progress(written)
				})
			})
		})
		if err != nil {
			return err
		}
	}

	// A zero range, stored as no block, arrives with the copy.
	written.Done = written.Total
	progress(written)
	return nil
}

// copyFile copies the file name from part into tree, calling report with
// the size of each chunk written.
func copyFile(part fs.FS, tree *os.Root, name string, report func(uint64)) error {
	// Open the source and a new destination file.
	src, err := part.Open(name)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := tree.MkdirAll(path.Dir(name), 0o755); err != nil {
		return err
	}
	dst, err := tree.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	// Copy in chunks so progress moves within a large file.
	buf := make([]byte, 1<<20)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				_ = dst.Close()
				return err
			}
			report(uint64(n))
		}
		if err == io.EOF {
			return dst.Close()
		}
		if err != nil {
			_ = dst.Close()
			return err
		}
	}
}
