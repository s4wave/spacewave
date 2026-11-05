package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_sftp "github.com/s4wave/spacewave/db/block/store/sftp"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/sirupsen/logrus"
)

const (
	// sftpPasswordEnv names the variable holding the SFTP password.
	sftpPasswordEnv = "PACK_COPY_SFTP_PASSWORD" //nolint:gosec // the name of a variable, not a password.
	// keyIDEnv names the variable holding the bucket's key id.
	keyIDEnv = "PACK_COPY_KEY_ID"
	// keyEnv names the variable holding the bucket's key.
	keyEnv = "PACK_COPY_KEY"
	// graphDir holds the graph stream under the target prefix.
	graphDir = "graph/"
)

// Scratch file prefixes. Each view owns every key under its prefix.
var (
	// sourceIndexPrefix holds the source's pack index.
	sourceIndexPrefix = []byte("src/")
	// targetIndexPrefix holds the target's pack index.
	targetIndexPrefix = []byte("dst/")
	// proofsPrefix holds the proofs.
	proofsPrefix = []byte("proof/")
	// meterKey holds the bytes read from the source, across runs.
	meterKey = []byte("meter/used")
)

// Args configures the commands.
type Args struct {
	// Scratch is the scratch file.
	Scratch string
	// Width is the packs indexed at once.
	Width int
	// BudgetBytes is the bytes the source may serve across runs, zero for no
	// limit.
	BudgetBytes uint64

	// SFTPAddr is the source server's host:port.
	SFTPAddr string
	// SFTPUser is the source login.
	SFTPUser string
	// SFTPHostKey pins the source server.
	SFTPHostKey string
	// SFTPConns caps the connections to the source.
	SFTPConns int
	// SFTPRoot is the source directory, relative to the login directory.
	SFTPRoot string
	// SourcePrefix starts the source's keys.
	SourcePrefix string

	// Bucket is the target bucket.
	Bucket string
	// Endpoint is the target S3 endpoint.
	Endpoint string
	// Region is the target S3 region.
	Region string
	// TargetPrefix starts the target's keys.
	TargetPrefix string

	// Roots are the blocks to copy, as b58 refs.
	Roots cli.StringSlice
	// UnknownRefs is the file listing blocks whose pack entry lacks true refs.
	UnknownRefs string
	// Frames are block frame files pulled from production.
	Frames cli.StringSlice
	// Holes is the holes list a pass writes.
	Holes string
	// Store names the block store in the holes list.
	Store string
	// Reads is the reads kept in flight.
	Reads int
	// Uploads is the packs written at once.
	Uploads int

	// Archive configures the archive peer source.
	Archive archiveArgs
}

// storeFlags binds the flags both commands share.
func (a *Args) storeFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "scratch", Usage: "scratch file holding the indexes, proofs and byte count", Destination: &a.Scratch, Required: true},
		&cli.IntFlag{Name: "width", Usage: "packs indexed at once", Value: 2, Destination: &a.Width},
		&cli.Uint64Flag{Name: "budget-bytes", Usage: "bytes the source may serve across runs; zero has no limit", Destination: &a.BudgetBytes},
		&cli.StringFlag{Name: "sftp-addr", Usage: "source server's host:port", Destination: &a.SFTPAddr, Required: true},
		&cli.StringFlag{Name: "sftp-user", Usage: "source login; the password comes from " + sftpPasswordEnv, Destination: &a.SFTPUser, Required: true},
		&cli.StringFlag{Name: "sftp-host-key", Usage: "source server's host key line or SHA256 fingerprint", Destination: &a.SFTPHostKey, Required: true},
		&cli.IntFlag{Name: "sftp-conns", Usage: "most connections to the source", Value: 2, Destination: &a.SFTPConns},
		&cli.StringFlag{Name: "sftp-root", Usage: "source directory holding the source prefix", Destination: &a.SFTPRoot, Required: true},
		&cli.StringFlag{Name: "source-prefix", Usage: "key prefix of the source's packs", Destination: &a.SourcePrefix},
		&cli.StringFlag{Name: "bucket", Usage: "target bucket; the key comes from " + keyIDEnv + " and " + keyEnv, Destination: &a.Bucket, Required: true},
		&cli.StringFlag{Name: "endpoint", Usage: "target S3 endpoint", Value: "https://s3.us-east-005.backblazeb2.com", Destination: &a.Endpoint},
		&cli.StringFlag{Name: "region", Usage: "target S3 region", Value: "us-east-005", Destination: &a.Region},
		&cli.StringFlag{Name: "target-prefix", Usage: "key prefix of the clean store", Value: "world/v2/", Destination: &a.TargetPrefix},
	}
}

// copyFlags binds the flags of the copy command.
func (a *Args) copyFlags() []cli.Flag {
	return append([]cli.Flag{
		&cli.StringSliceFlag{Name: "root", Usage: "block to copy, as a b58 ref; repeat for each", Destination: &a.Roots, Required: true},
		&cli.StringFlag{Name: "unknown-refs", Usage: "file listing blocks whose pack entry lacks their true refs", Destination: &a.UnknownRefs},
		&cli.StringSliceFlag{Name: "frames", Usage: "block frame file pulled from production; repeat for each", Destination: &a.Frames},
		&cli.StringFlag{Name: "holes", Usage: "holes list to write, which block-export takes as it is", Value: "holes.txt", Destination: &a.Holes},
		&cli.StringFlag{Name: "store", Usage: "block store name in the holes list", Value: "hyperline-world", Destination: &a.Store},
		&cli.IntFlag{Name: "reads", Usage: "block reads kept in flight", Value: packcopy.DefaultReads, Destination: &a.Reads},
		&cli.IntFlag{Name: "uploads", Usage: "packs uploaded at once; each holds about 100 MiB of memory", Value: 8, Destination: &a.Uploads},
	}, a.Archive.flags()...)
}

// run holds what a command opened.
type run struct {
	// le logs the run.
	le *logrus.Entry
	// scratch is the scratch file.
	scratch *s4db.DB
	// meter counts the source's bytes.
	meter *packcopy.Meter
	// sftp reaches the source.
	sftp *block_store_sftp.Client
	// objects reaches the target bucket.
	objects *block_store_s3.Client
	// source is the source's pack store.
	source *block_store_s3.PackStore
	// target is the target's pack store.
	target *block_store_s3.PackStore
}

// open opens the scratch file and both stores.
func (a *Args) open() (_ *run, rerr error) {
	// Open the scratch file.
	r := &run{le: logrus.NewEntry(logrus.StandardLogger())}
	defer func() {
		if rerr != nil {
			r.close()
		}
	}()
	var err error
	if r.scratch, err = s4db.Open(a.Scratch, s4db.Options{}); err != nil {
		return nil, err
	}

	// Reach the source within its connection cap and byte budget.
	r.sftp, err = block_store_sftp.NewClient(block_store_sftp.Config{
		Addr:     a.SFTPAddr,
		User:     a.SFTPUser,
		Password: os.Getenv(sftpPasswordEnv),
		HostKey:  a.SFTPHostKey,
		Conns:    a.SFTPConns,
	})
	if err != nil {
		return nil, err
	}
	used, err := readUsed(r.scratch)
	if err != nil {
		return nil, err
	}
	r.meter = packcopy.NewMeter(r.sftp, a.BudgetBytes, used)
	r.source = block_store_s3.NewPackStore(r.le, r.meter, a.SFTPRoot, a.SourcePrefix, &block_store_s3.PackStoreOptions{
		KeepPacks: true,
		Index:     kvtx_prefixer.NewPrefixer(r.scratch, sourceIndexPrefix),
		// A read over SFTP pays for bytes, not requests: fetch only the block.
		ExactReads: true,
	})

	// Reach the target. Tail merges stay off: they delete a pack, and a proof
	// must keep meaning its blocks are durable.
	keyID, key := os.Getenv(keyIDEnv), os.Getenv(keyEnv)
	if keyID == "" || key == "" {
		return nil, errors.Errorf("set %s and %s", keyIDEnv, keyEnv)
	}
	endpoint, insecure := strings.CutPrefix(a.Endpoint, "http://")
	r.objects, err = block_store_s3.BuildClient(&block_store_s3.ClientConfig{
		Endpoint:    strings.TrimPrefix(endpoint, "https://"),
		Region:      a.Region,
		DisableSsl:  insecure,
		Credentials: &block_store_s3.Credentials{AccessKeyId: keyID, SecretAccessKey: key},
	})
	if err != nil {
		return nil, err
	}
	r.target = block_store_s3.NewPackStore(r.le, r.objects, a.Bucket, a.TargetPrefix, &block_store_s3.PackStoreOptions{
		KeepPacks: true,
		Index:     kvtx_prefixer.NewPrefixer(r.scratch, targetIndexPrefix),
	})
	return r, nil
}

// close releases what open opened, saving the byte count.
func (r *run) close() {
	if r.target != nil {
		r.target.Close()
	}
	if r.source != nil {
		r.source.Close()
	}
	if r.sftp != nil {
		r.sftp.Close()
	}
	if r.scratch != nil {
		if r.meter != nil {
			if err := writeUsed(r.scratch, r.meter.Used()); err != nil {
				r.le.WithError(err).Warn("save source byte count")
			}
		}
		_ = r.scratch.Close()
	}
}

// buildIndexes brings both stores' indexes up to date.
func (a *Args) buildIndexes(ctx context.Context, r *run) error {
	// Index the source, logging about once a second.
	start := time.Now()
	report := func(p block_store_s3.IndexProgress) {
		r.le.WithFields(logrus.Fields{"packs": p.Packs, "keys": p.Keys, "bytes": p.Bytes, "used": r.meter.Used()}).Info("indexing")
	}
	if err := r.source.BuildIndex(ctx, a.Width, report); err != nil {
		return errors.Wrap(err, "index source")
	}

	// Index the target, which holds the packs of earlier runs.
	if err := r.target.BuildIndex(ctx, a.Width, nil); err != nil {
		return errors.Wrap(err, "index target")
	}
	r.le.WithField("took", time.Since(start).String()).Info("indexes current")
	return nil
}

// runIndex builds the scratch index.
func (a *Args) runIndex(*cli.Context) error {
	// Stop on a signal.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Open the stores and index them.
	r, err := a.open()
	if err != nil {
		return err
	}
	defer r.close()
	return a.buildIndexes(ctx, r)
}

// runCopy runs one pass.
func (a *Args) runCopy(*cli.Context) error {
	// Stop on a signal. The walk drains and its last pack still lands.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	r, err := a.open()
	if err != nil {
		return err
	}
	defer r.close()
	if err := a.buildIndexes(ctx, r); err != nil {
		return err
	}

	// Parse the roots.
	values := a.Roots.Value()
	roots := make([]*block.BlockRef, len(values))
	for i, value := range values {
		if roots[i], err = block.UnmarshalBlockRefB58(value); err != nil {
			return errors.Wrap(err, "parse --root")
		}
	}

	// Build the sources: the packs, then what production answered.
	sources, closeSources, err := a.openSources(ctx, r)
	if err != nil {
		return err
	}
	defer closeSources()

	// Write the holes list afresh: it names what this pass could not serve.
	holes, err := os.Create(a.Holes)
	if err != nil {
		return err
	}
	defer holes.Close()
	holeLines := bufio.NewWriter(holes)
	writeHole := func(ref *block.BlockRef, reason string) error {
		line := a.Store + " " + hex.EncodeToString(packfile.BlockKey(ref.GetHash())) + " " + reason + "\n"
		_, err := holeLines.WriteString(line)
		return err
	}

	// Run the pass, saving the byte count with each pack.
	result, err := packcopy.Copy(ctx, &packcopy.Options{
		Roots:   roots,
		Target:  r.target,
		Sources: sources,
		Proofs:  kvtx_prefixer.NewPrefixer(r.scratch, proofsPrefix),
		Graph:   packcopy.NewGraphStream(r.objects, a.Bucket, a.TargetPrefix+graphDir),
		Holes:   writeHole,
		Reads:   a.Reads,
		Uploads: a.Uploads,
		Progress: func(p packcopy.Result) {
			if werr := writeUsed(r.scratch, r.meter.Used()); werr != nil {
				r.le.WithError(werr).Warn("save source byte count")
			}
			r.le.WithFields(resultFields(&p)).Info("pack written")
		},
	})
	if ferr := holeLines.Flush(); ferr != nil && err == nil {
		err = ferr
	}
	if result != nil {
		r.le.WithFields(resultFields(result)).WithField("used", r.meter.Used()).Info("pass finished")
	}
	return err
}

// openSources builds the source list and the function that closes it.
func (a *Args) openSources(ctx context.Context, r *run) ([]packcopy.Source, func(), error) {
	// Mark the listed blocks in the packs.
	var packs packcopy.Source = r.source
	if a.UnknownRefs != "" {
		list, err := os.Open(a.UnknownRefs)
		if err != nil {
			return nil, nil, err
		}
		defer list.Close()
		if packs, err = packcopy.NewUnknownRefs(r.source, list); err != nil {
			return nil, nil, err
		}
	}
	sources := []packcopy.Source{packs}

	// Add the pulled blocks.
	frames, err := packcopy.OpenFrameStore(a.Frames.Value())
	if err != nil {
		return nil, nil, err
	}
	sources = append(sources, frames)

	// Add the archive peer, which answers last.
	closeArchive := func() {}
	if a.Archive.enabled() {
		archive, release, err := a.Archive.open(ctx, r.le)
		if err != nil {
			frames.Close()
			return nil, nil, err
		}
		sources = append(sources, archive)
		closeArchive = release
	}
	return sources, func() {
		closeArchive()
		frames.Close()
	}, nil
}

// resultFields describes a result.
func resultFields(r *packcopy.Result) logrus.Fields {
	fields := logrus.Fields{
		"read":     r.Read,
		"bytes":    r.ReadBytes,
		"held":     r.Held,
		"written":  r.Written,
		"packs":    r.Packs,
		"proven":   r.Proven,
		"complete": r.Complete,
	}
	for reason, count := range r.Holes {
		fields["holes-"+reason] = count
	}
	return fields
}

// readUsed returns the bytes earlier runs read from the source.
func readUsed(store kvtx.Store) (uint64, error) {
	// Open a read.
	ctx := context.Background()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return 0, err
	}
	defer tx.Discard()

	// Read the count, zero on a first run.
	data, found, err := tx.Get(ctx, meterKey)
	if err != nil || !found {
		return 0, err
	}
	used, _ := binary.Uvarint(data)
	return used, nil
}

// writeUsed saves the bytes read from the source.
func writeUsed(store kvtx.Store, used uint64) error {
	// Open a write.
	ctx := context.Background()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Save the count.
	if err := tx.Set(ctx, meterKey, binary.AppendUvarint(nil, used)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
