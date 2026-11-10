package main

import (
	"bufio"
	"cmp"
	"context"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
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
	block_store_sftp "github.com/s4wave/spacewave/db/block/store/sftp"
	"github.com/s4wave/spacewave/db/block/writelog"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/sirupsen/logrus"
)

const (
	// sftpPasswordEnv names the variable holding the SFTP password.
	sftpPasswordEnv = "PACK_SWEEP_SFTP_PASSWORD" //nolint:gosec // the name of a variable, not a password.
	// defaultIndexTable is the table of a pack index file.
	defaultIndexTable = "pack_index"
)

// Scratch file keys. The sweep owns every key under its prefix.
var (
	// sweepPrefix holds the sweep's catalog and cycle.
	sweepPrefix = []byte("sweep/")
	// storeKey holds the store the catalog describes.
	storeKey = []byte("store")
)

// Args configures the commands.
type Args struct {
	// Scratch is the scratch file.
	Scratch string

	// StoreID is the block store whose pack index the index file holds.
	StoreID string
	// Index is a copy of the store's pack index file, read for the pack list.
	Index string
	// IndexType is the index file's volume type; empty detects it.
	IndexType string
	// IndexTable is the table of a SQLite index file.
	IndexTable string
	// IndexBucket is the bucket of a Bolt index file.
	IndexBucket string
	// SFTPAddr is the store server's host:port.
	SFTPAddr string
	// SFTPUser is the store login.
	SFTPUser string
	// SFTPHostKey pins the store server.
	SFTPHostKey string
	// SFTPConns caps the connections to the store.
	SFTPConns int
	// SFTPRoot is the store directory, relative to the login directory.
	SFTPRoot string
	// S3Endpoint is the S3 endpoint the snapshot reads instead of SFTP.
	S3Endpoint string
	// S3Region is the S3 bucket's region.
	S3Region string
	// S3Bucket is the S3 bucket holding the prefix.
	S3Bucket string
	// Prefix starts the store's keys.
	Prefix string
	// Overlay is the directory of a rehearsal's overlay, which records every
	// write, move and delete while the store is only read; empty reaches the
	// store itself.
	Overlay string
	// LocalPacks is a directory of pack copies, named by id, read first.
	LocalPacks string
	// Reads is the packs read at once.
	Reads int

	// Roots are the walk's roots, as b58 refs.
	Roots cli.StringSlice
	// RootsFile lists more roots, one b58 ref a line.
	RootsFile string
	// LogPosition is the write log's position at the roots.
	LogPosition string
	// Frames are block frame files serving blocks the catalog lacks.
	Frames cli.StringSlice
	// Batch is the frontier keys per walk transaction.
	Batch int

	// LogDir is the backend's write log directory, read and trimmed by the
	// pull.
	LogDir string
	// PullEvery is how often a following pull reads the log.
	PullEvery time.Duration
	// Follow keeps the pull reading the log until a signal stops it.
	Follow bool

	// MinAge is the youngest pack judged.
	MinAge time.Duration
	// LowPercent is the live share at or under which a pack is low.
	LowPercent int
	// Report is the report file, one judgment a line.
	Report string
	// Holes is the holes file, one hole a line.
	Holes string
	// AcceptMissing lists missing blocks accepted as lost, one b58 ref a line.
	AcceptMissing string
	// AdminURL is the backend's pack sweep admin service, a ws or wss URL.
	AdminURL string
	// Wait is how long a pack stays quarantined before its recheck can pass
	// it.
	Wait time.Duration
	// Moves is the packs the trash moves at once.
	Moves int
	// Packs names packs the restore brings back besides those reads missed.
	Packs cli.StringSlice
	// MaxPackBytes bounds a pack the compaction writes.
	MaxPackBytes int64

	// dialAdmin reaches the admin service; nil dials AdminURL. Tests set it.
	dialAdmin func(ctx context.Context) (*block_packsweep_admin.Client, func(), error)
	// dial reaches the store; nil dials SFTP. Tests set it.
	dial func() (block_store_s3.ObjectStore, string, func(), error)
	// out receives the printed results; nil prints to stdout.
	out io.Writer
}

// scratchFlags binds the flag every command takes.
func (a *Args) scratchFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "scratch", Usage: "scratch file holding the catalog and the cycle", Destination: &a.Scratch, Required: true},
	}
}

// storeFlags binds the flags of the snapshot.
func (a *Args) storeFlags() []cli.Flag {
	flags := append(a.storeSourceFlags(false), a.s3Flags()...)
	return append(flags,
		&cli.StringFlag{Name: "index", Usage: "copy of the store's pack index file, read for the pack list", Destination: &a.Index, Required: true},
		&cli.StringFlag{Name: "index-type", Usage: "volume type of the pack index file: sqlite, bolt or s4db; detected when unset", Destination: &a.IndexType},
		&cli.StringFlag{Name: "index-table", Usage: "table of a SQLite pack index file", Value: defaultIndexTable, Destination: &a.IndexTable},
		&cli.StringFlag{Name: "index-bucket", Usage: "bucket of a bbolt pack index file", Value: packcopy.DefaultIndexBucket, Destination: &a.IndexBucket},
		&cli.StringFlag{Name: "local-packs", Usage: "directory of pack copies named by id, read before the store", Destination: &a.LocalPacks},
		&cli.IntFlag{Name: "reads", Usage: "packs read at once, each held whole in memory", Value: packsweep.DefaultPackReads, Destination: &a.Reads},
	)
}

// sftpFlags binds the flags that name and reach the store over SFTP.
func (a *Args) sftpFlags() []cli.Flag {
	return a.storeSourceFlags(true)
}

// storeSourceFlags binds the flags that name and reach the store, the SFTP
// ones required only when sftp is set: the snapshot may read S3 instead.
func (a *Args) storeSourceFlags(sftp bool) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "store-id", Usage: "block store id whose pack index the index file holds", Destination: &a.StoreID, Required: true},
		&cli.StringFlag{Name: "sftp-addr", Usage: "store server's host:port", Destination: &a.SFTPAddr, Required: sftp},
		&cli.StringFlag{Name: "sftp-user", Usage: "store login; the password comes from " + sftpPasswordEnv, Destination: &a.SFTPUser, Required: sftp},
		&cli.StringFlag{Name: "sftp-host-key", Usage: "store server's host key line or SHA256 fingerprint", Destination: &a.SFTPHostKey, Required: sftp},
		&cli.IntFlag{Name: "sftp-conns", Usage: "most SSH connections to the store at once", Value: 2, Destination: &a.SFTPConns},
		&cli.StringFlag{Name: "sftp-root", Usage: "store directory holding the prefix", Destination: &a.SFTPRoot, Required: sftp},
		&cli.StringFlag{Name: "prefix", Usage: "key prefix of the store's packs", Destination: &a.Prefix},
		&cli.StringFlag{Name: "overlay", Usage: "rehearse: only read the store, recording every write, move and delete in this directory, which the rehearsal's backend shares", Destination: &a.Overlay},
	}
}

// walkFlags binds the flags of the walk.
func (a *Args) walkFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{Name: "root", Usage: "root block, as a b58 ref; repeat for each", Destination: &a.Roots},
		&cli.StringFlag{Name: "roots-file", Usage: "file listing more roots, one b58 ref a line", Destination: &a.RootsFile},
		&cli.StringFlag{Name: "log-position", Usage: "the write log's position when the roots were read", Destination: &a.LogPosition},
		a.framesFlag(),
		&cli.IntFlag{Name: "batch", Usage: "frontier keys per transaction", Value: packsweep.DefaultWalkBatch, Destination: &a.Batch},
		a.logDirFlag(),
		a.pullEveryFlag(),
	}
}

// framesFlag binds the frame files serving blocks no pack holds.
func (a *Args) framesFlag() cli.Flag {
	return &cli.StringSliceFlag{Name: "frames", Usage: "block frame file serving blocks no pack holds; repeat for each", Destination: &a.Frames}
}

// logDirFlag binds the write log directory.
func (a *Args) logDirFlag() cli.Flag {
	return &cli.StringFlag{Name: "log-dir", Usage: "the backend's write log directory, pulled into the scratch file and trimmed", Destination: &a.LogDir}
}

// pullEveryFlag binds the interval of a following pull.
func (a *Args) pullEveryFlag() cli.Flag {
	return &cli.DurationFlag{Name: "pull-every", Usage: "how often a following pull reads the write log", Value: packsweep.DefaultPullEvery, Destination: &a.PullEvery}
}

// pullFlags binds the flags of the log pull.
func (a *Args) pullFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "log-dir", Usage: "the backend's write log directory, pulled into the scratch file and trimmed", Destination: &a.LogDir, Required: true},
		&cli.StringFlag{Name: "log-position", Usage: "the write log's position when the roots were read; the walk's when unset", Destination: &a.LogPosition},
		&cli.BoolFlag{Name: "follow", Usage: "keep pulling until a signal or a gap stops it", Destination: &a.Follow},
		a.pullEveryFlag(),
	}
}

// closureFlags binds the flags of the closure.
func (a *Args) closureFlags() []cli.Flag {
	return []cli.Flag{
		a.framesFlag(),
		&cli.IntFlag{Name: "batch", Usage: "frontier keys per transaction", Value: packsweep.DefaultWalkBatch, Destination: &a.Batch},
	}
}

// judgeFlags binds the flags of the judgment.
func (a *Args) judgeFlags() []cli.Flag {
	return []cli.Flag{
		a.framesFlag(),
		a.logDirFlag(),
		&cli.DurationFlag{Name: "min-age", Usage: "youngest pack judged, before the snapshot", Value: packsweep.DefaultMinAge, Destination: &a.MinAge},
		&cli.IntFlag{Name: "low-percent", Usage: "live share of value bytes at or under which a pack is low; negative for none", Value: packsweep.DefaultLowLivePercent, Destination: &a.LowPercent},
		&cli.StringFlag{Name: "report", Usage: "report file to write, one JSON judgment a line", Value: "report.jsonl", Destination: &a.Report},
		&cli.StringFlag{Name: "holes", Usage: "holes file to write, one b58 ref and reason a line", Value: "holes.txt", Destination: &a.Holes},
		&cli.StringFlag{Name: "accept-missing", Usage: "file listing missing blocks accepted as lost, one b58 ref a line", Destination: &a.AcceptMissing},
	}
}

// openScratch opens the scratch file and the sweep's view of it.
func (a *Args) openScratch() (*s4db.DB, kvtx.Store, error) {
	db, err := s4db.Open(a.Scratch, s4db.Options{})
	if err != nil {
		return nil, nil, err
	}
	return db, kvtx_prefixer.NewPrefixer(db, sweepPrefix), nil
}

// signalContext stops the step on a signal; it resumes from its last batch.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// print writes the value fn builds as one line of JSON.
func (a *Args) print(fn func(*fastjson.Arena) *fastjson.Value) error {
	var arena fastjson.Arena
	line := append(fn(&arena).MarshalTo(nil), '\n')
	_, err := cmp.Or[io.Writer](a.out, os.Stdout).Write(line)
	return err
}

// runNewCycle starts a cycle.
func (a *Args) runNewCycle(*cli.Context) error {
	// Open the scratch file until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()

	// Start the cycle and print its header.
	head, err := packsweep.NewCycle(ctx, store)
	if err != nil {
		return err
	}
	return a.print(head.JSON)
}

// runSnapshot takes the cycle's snapshot and reads the packs the catalog
// lacks.
func (a *Args) runSnapshot(*cli.Context) error {
	// Open the scratch file until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	le := logrus.NewEntry(logrus.StandardLogger())
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()

	// Reach the store, and refuse a scratch file another store's catalog
	// fills: its keys would judge this store's packs.
	client, root, closeClient, err := a.dialStore()
	if err != nil {
		return err
	}
	defer closeClient()
	if err := bindStore(ctx, db, a.storeBinding(root)); err != nil {
		return err
	}

	// Open the store on the index copy, which lists the packs without a
	// listing of the directory.
	if _, err := os.Stat(a.Index); err != nil {
		return errors.Wrap(err, "--index")
	}
	index, closeIndex, err := packcopy.OpenIndexFile(ctx, a.Index, &packcopy.IndexFileOptions{
		Kind:   a.IndexType,
		Table:  cmp.Or(a.IndexTable, defaultIndexTable),
		Bucket: a.IndexBucket,
		SQLite: store_kvtx_sqlite.Pragmas{WithoutRowID: true},
	})
	if err != nil {
		return errors.Wrap(err, "open --index")
	}
	defer closeIndex()

	// Read the store as its only writer would, exactly the bytes each pack
	// row names.
	packStore := block_store_s3.NewPackStore(le, client, root, a.Prefix, &block_store_s3.PackStoreOptions{
		KeepPacks:  true,
		Index:      kvtx_prefixer.NewPrefixer(index, block_store_s3.PackIndexPrefix(a.StoreID)),
		SoleWriter: true,
		ExactReads: true,
	})
	defer packStore.Close()

	// Serve the packs from the local copies where they match.
	var packs packsweep.PackSource = packStore
	var local *packsweep.DirPacks
	if a.LocalPacks != "" {
		local = &packsweep.DirPacks{PackSource: packStore, Dir: a.LocalPacks}
		packs = local
	}

	// Read the packs, logging each.
	start := time.Now()
	result, err := packsweep.Snapshot(ctx, &packsweep.SnapshotOptions{
		Store: store,
		Packs: packs,
		Reads: a.Reads,
		Progress: func(p packsweep.SnapshotProgress) {
			le.WithFields(logrus.Fields{
				"packs":      p.Packs,
				"catalogued": p.Catalogued,
				"skipped":    p.Skipped,
				"bytes":      p.Bytes,
			}).Info("pack catalogued")
		},
	})
	if err != nil {
		return err
	}

	// Log the time and the local reads, and print the result.
	fields := logrus.Fields{"took": time.Since(start).String()}
	if local != nil {
		fields["local"] = local.Local.Load()
	}
	le.WithFields(fields).Info("snapshot complete")
	return a.print(result.JSON)
}

// dialStore reaches the store, through the rehearsal's overlay when one is
// named.
func (a *Args) dialStore() (block_store_s3.ObjectStore, string, func(), error) {
	// Reach the store.
	objects, root, closeStore, err := a.dialBase()
	if err != nil || a.Overlay == "" {
		return objects, root, closeStore, err
	}

	// Lay the overlay over it.
	ov, err := block_store_s3_overlay.New(objects, root, a.Overlay)
	if err != nil {
		closeStore()
		return nil, "", nil, errors.Wrap(err, "--overlay")
	}
	return ov, root, closeStore, nil
}

// dialBase reaches the store over SFTP, or read-only over S3 when the S3
// flags name it, unless a test set another way.
func (a *Args) dialBase() (block_store_s3.ObjectStore, string, func(), error) {
	// Take a test's way, or the S3 bucket the flags name.
	if a.dial != nil {
		return a.dial()
	}
	if a.usesS3() {
		return a.dialS3()
	}

	// Reach the store over SFTP, named in full.
	if err := a.checkSFTP(); err != nil {
		return nil, "", nil, err
	}
	client, err := block_store_sftp.NewClient(block_store_sftp.Config{
		Addr:     a.SFTPAddr,
		User:     a.SFTPUser,
		Password: os.Getenv(sftpPasswordEnv),
		HostKey:  a.SFTPHostKey,
		Conns:    a.SFTPConns,
	})
	if err != nil {
		return nil, "", nil, err
	}
	return client, a.SFTPRoot, client.Close, nil
}

// runWalk walks what the roots reach.
func (a *Args) runWalk(*cli.Context) error {
	// Open the scratch file until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	le := logrus.NewEntry(logrus.StandardLogger())
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()

	// Read the roots.
	values := a.Roots.Value()
	if a.RootsFile != "" {
		lines, err := readRefLines(a.RootsFile)
		if err != nil {
			return errors.Wrap(err, "--roots-file")
		}
		values = append(values, lines...)
	}
	if len(values) == 0 {
		return errors.New("name the roots with --root or --roots-file")
	}

	// Parse each root.
	roots := make([]*block.BlockRef, len(values))
	for i, value := range values {
		if roots[i], err = block.UnmarshalBlockRefB58(value); err != nil {
			return errors.Wrap(err, "parse root "+value)
		}
	}

	// Serve the blocks no catalogued pack holds from the frame files, and
	// pull the log while the walk runs, keeping the backend's copy small.
	refs, closeFrames, err := a.openFrames()
	if err != nil {
		return err
	}
	defer closeFrames()
	ctx, stopPull, err := a.followDuring(ctx, le, store)
	if err != nil {
		return err
	}

	// Walk, logging every batch.
	start := time.Now()
	head, err := packsweep.Walk(ctx, &packsweep.WalkOptions{
		Store:       store,
		Roots:       roots,
		LogPosition: a.LogPosition,
		Refs:        refs,
		Batch:       a.Batch,
		Progress: func(p packsweep.WalkProgress) {
			le.WithFields(logrus.Fields{"live": p.Live, "batches": p.Batches}).Info("walk batch")
		},
	})
	if err := cmp.Or(stopPull(), err); err != nil {
		return err
	}

	// Log the time and print the header.
	le.WithField("took", time.Since(start).String()).Info("walk complete")
	return a.print(head.JSON)
}

// openFrames opens the frame files serving blocks no catalogued pack holds,
// nil when none are named.
func (a *Args) openFrames() (packsweep.RefSource, func(), error) {
	// Open the named files.
	paths := a.Frames.Value()
	if len(paths) == 0 {
		return nil, func() {}, nil
	}
	frames, err := packcopy.OpenFrameStore(paths)
	if err != nil {
		return nil, nil, err
	}
	return packsweep.SourceRefs{frames}, frames.Close, nil
}

// pullOptions builds the pull of the log directory from start.
func (a *Args) pullOptions(store kvtx.Store, start string) *packsweep.PullOptions {
	return &packsweep.PullOptions{Store: store, Source: writelog.Dir(a.LogDir), Start: start}
}

// followDuring pulls the log in the background from --log-position while
// the step the returned context bounds runs. stop ends the pull and returns
// its error; a failed pull cancels the step.
func (a *Args) followDuring(ctx context.Context, le *logrus.Entry, store kvtx.Store) (context.Context, func() error, error) {
	// Pull only with a log directory, which needs the position R.
	if a.LogDir == "" {
		return ctx, func() error { return nil }, nil
	}
	if a.LogPosition == "" {
		return nil, nil, errors.New("--log-dir needs --log-position, the position the roots were read at")
	}

	// Follow until stopped, cancelling the step on a failure.
	stepCtx, cancelStep := context.WithCancel(ctx)
	pullCtx, cancelPull := context.WithCancel(stepCtx)
	done := make(chan error, 1)
	go func() {
		pull, err := packsweep.FollowLog(pullCtx, a.pullOptions(store, a.LogPosition), a.PullEvery)
		switch {
		case err != nil && pullCtx.Err() == nil:
			cancelStep()
		case err == nil && pull.Gap != "":
			le.WithField("gap", pull.Gap).Warn("the write log stopped covering the cycle")
		}
		done <- err
	}()

	// Stop the pull, keeping a failure that did not come from the stop.
	return stepCtx, func() error {
		// Stop the pull and the step's context.
		cancelPull()
		err := <-done
		cancelStep()
		if pullCtx.Err() != nil && errors.Is(err, context.Canceled) {
			err = nil
		}
		return errors.Wrap(err, "pull the write log")
	}, nil
}

// runPullLog stores the log's lines since the last pull in the scratch file
// and trims them from the log directory.
func (a *Args) runPullLog(*cli.Context) error {
	// Open the scratch file until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()

	// Pull from the walk's R unless one is given.
	start := a.LogPosition
	if start == "" {
		head, err := packsweep.ReadHead(ctx, store)
		if err != nil {
			return err
		}
		start = head.LogPosition
	}

	// Pull once, or until a signal or a gap.
	opts := a.pullOptions(store, start)
	if a.Follow {
		_, err = packsweep.FollowLog(ctx, opts, a.PullEvery)
	} else {
		_, err = packsweep.PullLog(ctx, opts)
	}
	if err != nil && ctx.Err() == nil {
		return err
	}

	// Print the pull's row.
	pull, err := packsweep.ReadLogPull(context.WithoutCancel(ctx), store)
	if err != nil || pull == nil {
		return cmp.Or(err, errors.New("no log pull"))
	}
	return a.print(pull.JSON)
}

// runClosure walks what the pulled log keys reach that the roots do not.
func (a *Args) runClosure(*cli.Context) error {
	// Open the scratch file and the frame files until a signal stops the
	// step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()
	refs, closeFrames, err := a.openFrames()
	if err != nil {
		return err
	}
	defer closeFrames()

	// Walk W and print the header.
	head, err := packsweep.Closure(ctx, &packsweep.ClosureOptions{Store: store, Refs: refs, Batch: a.Batch})
	if err != nil {
		return err
	}
	return a.print(head.JSON)
}

// judgeResult is what the judgment prints.
type judgeResult struct {
	// summary totals the verdicts.
	summary *packsweep.Summary
	// holes counts the walk's holes by reason.
	holes map[string]uint64
	// blocking counts the holes that stop this cycle from removing anything.
	blocking map[string]uint64
}

// JSON describes the result; removable reports whether the cycle may go on to
// quarantine.
func (r *judgeResult) JSON(a *fastjson.Arena) *fastjson.Value {
	// Describe the counts.
	o := a.NewObject()
	o.Set("summary", r.summary.JSON(a))
	o.Set("holes", packsweep.JSONCounts(a, r.holes))
	o.Set("blocking", packsweep.JSONCounts(a, r.blocking))

	// The cycle is removable once no hole blocks it.
	if len(r.blocking) == 0 {
		o.Set("removable", a.NewTrue())
	} else {
		o.Set("removable", a.NewFalse())
	}
	return o
}

// runJudge judges the packs and writes the report and the holes.
func (a *Args) runJudge(*cli.Context) error {
	// Open the scratch file until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()

	// Store the log's last lines and walk what they reach first.
	if err := a.closeOver(ctx, store); err != nil {
		return err
	}

	// Judge every snapshot pack.
	sum, err := packsweep.Judge(ctx, &packsweep.JudgeOptions{
		Store:          store,
		MinAge:         a.MinAge,
		LowLivePercent: a.LowPercent,
	})
	if err != nil {
		return err
	}

	// Write the report, one judgment a line.
	if err := writeLines(a.Report, func(w *bufio.Writer) error {
		var arena fastjson.Arena
		var line []byte
		return packsweep.Judgments(ctx, store, func(j *packsweep.Judgment) error {
			arena.Reset()
			line = append(j.JSON(&arena).MarshalTo(line[:0]), '\n')
			_, err := w.Write(line)
			return err
		})
	}); err != nil {
		return errors.Wrap(err, "write --report")
	}

	// Write the holes.
	var holes []packsweep.Hole
	if err := writeLines(a.Holes, func(w *bufio.Writer) error {
		return packsweep.Holes(ctx, store, func(h packsweep.Hole) error {
			holes = append(holes, h)
			_, err := w.WriteString(h.Ref.MarshalString() + " " + h.Reason + "\n")
			return err
		})
	}); err != nil {
		return errors.Wrap(err, "write --holes")
	}

	// Read the holes the operator accepted.
	accepted := make(map[string]bool)
	if a.AcceptMissing != "" {
		lines, err := readRefLines(a.AcceptMissing)
		if err != nil {
			return errors.Wrap(err, "--accept-missing")
		}
		for _, line := range lines {
			accepted[line] = true
		}
	}

	// Count the holes the accepted list does not cover, with a log that does
	// not cover the cycle, and print the result.
	head, err := packsweep.ReadHead(ctx, store)
	if err != nil {
		return err
	}
	pull, err := packsweep.ReadLogPull(ctx, store)
	if err != nil {
		return err
	}
	result := &judgeResult{
		summary:  sum,
		holes:    head.Holes,
		blocking: packsweep.BlockingHoles(head, accepted, holes),
	}
	if reason := packsweep.LogBlocking(head, pull); reason != "" {
		result.blocking[reason] = 1
	}
	return a.print(result.JSON)
}

// closeOver pulls the log directory, when one is named, from the walk's R and
// walks W, so the judgment counts every key put since R.
func (a *Args) closeOver(ctx context.Context, store kvtx.Store) error {
	// Pull from the walk's R.
	if a.LogDir == "" {
		return nil
	}
	head, err := packsweep.ReadHead(ctx, store)
	if err != nil {
		return err
	}
	if head.LogPosition == "" {
		return errors.New("--log-dir needs a walk with --log-position")
	}
	if _, err := packsweep.PullLog(ctx, a.pullOptions(store, head.LogPosition)); err != nil {
		return err
	}

	// Walk what the pulled keys reach.
	refs, closeFrames, err := a.openFrames()
	if err != nil {
		return err
	}
	defer closeFrames()
	_, err = packsweep.Closure(ctx, &packsweep.ClosureOptions{Store: store, Refs: refs, Batch: a.Batch})
	return err
}

// runStatus prints the cycle's state.
func (a *Args) runStatus(*cli.Context) error {
	// Open the scratch file until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()

	// Read the cycle's header, its log pull and its counts.
	head, err := packsweep.ReadHead(ctx, store)
	if err != nil {
		return err
	}
	pull, err := packsweep.ReadLogPull(ctx, store)
	if err != nil {
		return err
	}
	records, err := recordCounts(ctx, store)
	if err != nil {
		return err
	}
	compacted, err := packsweep.CompactPacks(ctx, store)
	if err != nil {
		return err
	}

	// Print them.
	return a.print(func(arena *fastjson.Arena) *fastjson.Value {
		// Set the pull and the counts beside the header.
		v := head.JSON(arena)
		if pull != nil {
			v.Set("logPull", pull.JSON(arena))
		}
		v.Set("packRecords", packsweep.JSONCounts(arena, records))
		v.Set("compactPacks", packsweep.JSONCounts(arena, compacted))
		return v
	})
}

// writeLines writes a file through fn, replacing it only once fn succeeds.
func writeLines(path string, fn func(*bufio.Writer) error) error {
	// Create the file beside the path, removed unless renamed.
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	defer f.Close()

	// Write the lines through a buffer.
	w := bufio.NewWriter(f)
	if err := fn(w); err != nil {
		return err
	}

	// Flush and close the file, then move it over the path.
	if err := w.Flush(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readRefLines reads a file of b58 refs, one a line, skipping blank lines and
// lines starting with #. A line may carry more fields after the ref, as the
// holes file does.
func readRefLines(path string) ([]string, error) {
	// Read the file.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Take each line's first field, sorted with no repeats.
	var out []string
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ref, _, _ := strings.Cut(line, " ")
		out = append(out, ref)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// checkStore refuses a scratch file whose catalog describes a store other
// than id, or none.
func checkStore(ctx context.Context, store kvtx.Store, id string) error {
	// Open a read.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Compare the recorded store.
	data, found, err := tx.Get(ctx, storeKey)
	if err != nil {
		return err
	}
	if !found || string(data) != id {
		return errors.New("--scratch holds no catalog of this store id, root, prefix and overlay")
	}
	return nil
}

// storeBinding names the store the flags reach, its catalog's root on the
// server at root, and the rehearsal's overlay, so a rehearsal's scratch file
// never drives the store itself, nor the store's a rehearsal.
func (a *Args) storeBinding(root string) string {
	id := a.StoreID + "\x00" + root + "\x00" + a.Prefix
	if a.usesS3() {
		id = "s3\x00" + a.S3Endpoint + "\x00" + id
	}
	if a.Overlay != "" {
		dir, err := filepath.Abs(a.Overlay)
		if err != nil {
			dir = a.Overlay
		}
		id += "\x00overlay\x00" + dir
	}
	return id
}

// bindStore records the store the scratch file's catalog describes on its
// first snapshot, and refuses another.
func bindStore(ctx context.Context, store kvtx.Store, id string) error {
	// Open a write.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Compare the recorded store, or record this one.
	data, found, err := tx.Get(ctx, storeKey)
	if err != nil {
		return err
	}
	if found {
		if string(data) != id {
			return errors.New("--scratch holds the catalog of another store id, root, prefix or overlay; use a new scratch file")
		}
		return nil
	}
	if err := tx.Set(ctx, storeKey, []byte(id)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
