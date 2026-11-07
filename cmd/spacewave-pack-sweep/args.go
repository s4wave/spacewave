package main

import (
	"bufio"
	"cmp"
	"context"
	"io"
	"os"
	"os/signal"
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
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_sftp "github.com/s4wave/spacewave/db/block/store/sftp"
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
	// IndexTable is the index file's table.
	IndexTable string
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
	// Prefix starts the store's keys.
	Prefix string
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
	return []cli.Flag{
		&cli.StringFlag{Name: "store-id", Usage: "block store id whose pack index the index file holds", Destination: &a.StoreID, Required: true},
		&cli.StringFlag{Name: "index", Usage: "copy of the store's pack index file, read for the pack list", Destination: &a.Index, Required: true},
		&cli.StringFlag{Name: "index-table", Usage: "table of the pack index file", Value: defaultIndexTable, Destination: &a.IndexTable},
		&cli.StringFlag{Name: "sftp-addr", Usage: "store server's host:port", Destination: &a.SFTPAddr, Required: true},
		&cli.StringFlag{Name: "sftp-user", Usage: "store login; the password comes from " + sftpPasswordEnv, Destination: &a.SFTPUser, Required: true},
		&cli.StringFlag{Name: "sftp-host-key", Usage: "store server's host key line or SHA256 fingerprint", Destination: &a.SFTPHostKey, Required: true},
		&cli.IntFlag{Name: "sftp-conns", Usage: "most SSH connections to the store at once", Value: 2, Destination: &a.SFTPConns},
		&cli.StringFlag{Name: "sftp-root", Usage: "store directory holding the prefix", Destination: &a.SFTPRoot, Required: true},
		&cli.StringFlag{Name: "prefix", Usage: "key prefix of the store's packs", Destination: &a.Prefix},
		&cli.StringFlag{Name: "local-packs", Usage: "directory of pack copies named by id, read before the store", Destination: &a.LocalPacks},
		&cli.IntFlag{Name: "reads", Usage: "packs read at once, each held whole in memory", Value: packsweep.DefaultPackReads, Destination: &a.Reads},
	}
}

// walkFlags binds the flags of the walk.
func (a *Args) walkFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{Name: "root", Usage: "root block, as a b58 ref; repeat for each", Destination: &a.Roots},
		&cli.StringFlag{Name: "roots-file", Usage: "file listing more roots, one b58 ref a line", Destination: &a.RootsFile},
		&cli.StringFlag{Name: "log-position", Usage: "the write log's position when the roots were read", Destination: &a.LogPosition},
		&cli.StringSliceFlag{Name: "frames", Usage: "block frame file serving blocks no pack holds; repeat for each", Destination: &a.Frames},
		&cli.IntFlag{Name: "batch", Usage: "frontier keys per transaction", Value: packsweep.DefaultWalkBatch, Destination: &a.Batch},
	}
}

// judgeFlags binds the flags of the judgment.
func (a *Args) judgeFlags() []cli.Flag {
	return []cli.Flag{
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
	if err := bindStore(ctx, db, a.StoreID+"\x00"+root+"\x00"+a.Prefix); err != nil {
		return err
	}

	// Open the store on the index copy, which lists the packs without a
	// listing of the directory.
	if _, err := os.Stat(a.Index); err != nil {
		return errors.Wrap(err, "--index")
	}
	index, err := store_kvtx_sqlite.OpenWithPragmas(ctx, a.Index, cmp.Or(a.IndexTable, defaultIndexTable), store_kvtx_sqlite.Pragmas{WithoutRowID: true})
	if err != nil {
		return errors.Wrap(err, "open --index")
	}
	defer index.Close()

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
		fields["local"] = local.Local
	}
	le.WithFields(fields).Info("snapshot complete")
	return a.print(result.JSON)
}

// dialStore reaches the store over SFTP unless a test set another way.
func (a *Args) dialStore() (block_store_s3.ObjectStore, string, func(), error) {
	if a.dial != nil {
		return a.dial()
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

	// Serve the blocks no catalogued pack holds from the frame files.
	var refs packsweep.RefSource
	if paths := a.Frames.Value(); len(paths) != 0 {
		frames, err := packcopy.OpenFrameStore(paths)
		if err != nil {
			return err
		}
		defer frames.Close()
		refs = packsweep.SourceRefs{frames}
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
	if err != nil {
		return err
	}

	// Log the time and print the header.
	le.WithField("took", time.Since(start).String()).Info("walk complete")
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

	// Count the holes the accepted list does not cover, and print the result.
	head, err := packsweep.ReadHead(ctx, store)
	if err != nil {
		return err
	}
	result := &judgeResult{
		summary:  sum,
		holes:    head.Holes,
		blocking: packsweep.BlockingHoles(head, accepted, holes),
	}
	return a.print(result.JSON)
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

	// Print the cycle's header.
	head, err := packsweep.ReadHead(ctx, store)
	if err != nil {
		return err
	}
	return a.print(head.JSON)
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
			return errors.New("--scratch holds the catalog of another store id, root or prefix; use a new scratch file")
		}
		return nil
	}
	if err := tx.Set(ctx, storeKey, []byte(id)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
