package main

import (
	"bufio"
	"cmp"
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
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
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
	// defaultIndexTable is the table of a pack index file.
	defaultIndexTable = "pack_index"
)

// Scratch file prefixes. Each view owns every key under its prefix.
var (
	// proofsPrefix holds the proofs.
	proofsPrefix = []byte("proof/")
	// meterKey holds the bytes read from the source, across runs.
	meterKey = []byte("meter/used")
)

// Args configures the commands.
type Args struct {
	// Scratch is the scratch file.
	Scratch string
	// StoreID is the block store whose pack index the index files hold. The
	// clean store keeps the source's block store id under a new prefix.
	StoreID string
	// SourceIndex is a copy of the source's pack index file, which the source
	// store reads and may add rows to.
	SourceIndex string
	// TargetIndex is the clean store's pack index file, created if missing,
	// which an owner of the clean store opens as it is.
	TargetIndex string
	// IndexTable is the table of both index files.
	IndexTable string
	// Width is the target packs indexed at once.
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
	// PackMiB bounds one target pack in MiB; zero uses the store's default.
	PackMiB int64

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

	// Volume configures the Volume file sources.
	Volume volumeArgs
	// Archive configures the archive peer source.
	Archive archiveArgs
}

// storeFlags binds the flags both commands share.
func (a *Args) storeFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "scratch", Usage: "scratch file holding the proofs and byte count", Destination: &a.Scratch, Required: true},
		&cli.StringFlag{Name: "store-id", Usage: "block store id whose pack index the index files hold", Destination: &a.StoreID, Required: true},
		&cli.StringFlag{Name: "source-index", Usage: "copy of the source's pack index file; the source store may add rows", Destination: &a.SourceIndex, Required: true},
		&cli.StringFlag{Name: "target-index", Usage: "the clean store's pack index file, created if missing", Destination: &a.TargetIndex, Required: true},
		&cli.StringFlag{Name: "index-table", Usage: "table of both pack index files", Value: defaultIndexTable, Destination: &a.IndexTable},
		&cli.IntFlag{Name: "width", Usage: "target packs indexed at once", Value: 2, Destination: &a.Width},
		&cli.Uint64Flag{Name: "budget-bytes", Usage: "bytes the source may serve across runs; zero has no limit", Destination: &a.BudgetBytes},
		&cli.StringFlag{Name: "sftp-addr", Usage: "source server's host:port", Destination: &a.SFTPAddr, Required: true},
		&cli.StringFlag{Name: "sftp-user", Usage: "source login; the password comes from " + sftpPasswordEnv, Destination: &a.SFTPUser, Required: true},
		&cli.StringFlag{Name: "sftp-host-key", Usage: "source server's host key line or SHA256 fingerprint", Destination: &a.SFTPHostKey, Required: true},
		&cli.IntFlag{Name: "sftp-conns", Usage: "most SSH connections to the source at once, each carrying many reads", Value: 2, Destination: &a.SFTPConns},
		&cli.StringFlag{Name: "sftp-root", Usage: "source directory holding the source prefix", Destination: &a.SFTPRoot, Required: true},
		&cli.StringFlag{Name: "source-prefix", Usage: "key prefix of the source's packs", Destination: &a.SourcePrefix},
		&cli.StringFlag{Name: "bucket", Usage: "target bucket; the key comes from " + keyIDEnv + " and " + keyEnv, Destination: &a.Bucket, Required: true},
		&cli.StringFlag{Name: "endpoint", Usage: "target S3 endpoint", Value: "https://s3.us-east-005.backblazeb2.com", Destination: &a.Endpoint},
		&cli.StringFlag{Name: "region", Usage: "target S3 region", Value: "us-east-005", Destination: &a.Region},
		&cli.StringFlag{Name: "target-prefix", Usage: "key prefix of the clean store", Value: "world/v2/", Destination: &a.TargetPrefix},
		&cli.Int64Flag{Name: "pack-mib", Usage: "most MiB in one target pack, half of it block bytes; zero uses the store's default", Destination: &a.PackMiB},
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
	}, append(a.Volume.flags(), a.Archive.flags()...)...)
}

// run holds what a command opened.
type run struct {
	// le logs the run.
	le *logrus.Entry
	// scratch is the scratch file.
	scratch *s4db.DB
	// sourceIndex is the source's pack index file.
	sourceIndex store_kvtx_sqlite.Store
	// targetIndex is the target's pack index file.
	targetIndex store_kvtx_sqlite.Store
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

// open opens the scratch file, both index files and both stores.
func (a *Args) open(ctx context.Context) (_ *run, rerr error) {
	// Open the scratch file, closing what opened on a failure.
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

	// Open the index files. A row of the target index commits after its pack
	// is durable; a lost one is rebuilt from the pack.
	if _, err := os.Stat(a.SourceIndex); err != nil {
		return nil, errors.Wrap(err, "--source-index")
	}
	table := cmp.Or(a.IndexTable, defaultIndexTable)
	if r.sourceIndex, err = store_kvtx_sqlite.OpenWithPragmas(ctx, a.SourceIndex, table, store_kvtx_sqlite.Pragmas{WithoutRowID: true}); err != nil {
		return nil, errors.Wrap(err, "open --source-index")
	}
	if r.targetIndex, err = store_kvtx_sqlite.OpenWithPragmas(ctx, a.TargetIndex, table, store_kvtx_sqlite.Pragmas{FullSync: true, WithoutRowID: true}); err != nil {
		return nil, errors.Wrap(err, "open --target-index")
	}
	indexPrefix := block_store_s3.PackIndexPrefix(a.StoreID)

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

	// The source reads its blocks from the index alone: a listing of every
	// pack over two connections would cost hours, and each pass takes a fresh
	// copy of the index, which names the packs written since the last.
	r.source = block_store_s3.NewPackStore(r.le, r.meter, a.SFTPRoot, a.SourcePrefix, &block_store_s3.PackStoreOptions{
		KeepPacks:  true,
		Index:      kvtx_prefixer.NewPrefixer(r.sourceIndex, indexPrefix),
		SoleWriter: true,
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
		KeepPacks:    true,
		Index:        kvtx_prefixer.NewPrefixer(r.targetIndex, indexPrefix),
		SoleWriter:   true,
		MaxPackBytes: a.PackMiB << 20,
	})
	return r, nil
}

// close releases what open opened, saving the byte count.
func (r *run) close() {
	// Close the stores, then their connections and index files.
	if r.target != nil {
		r.target.Close()
	}
	if r.source != nil {
		r.source.Close()
	}
	if r.sftp != nil {
		r.sftp.Close()
	}
	for _, index := range []store_kvtx_sqlite.Store{r.targetIndex, r.sourceIndex} {
		if index != nil {
			_ = index.Close()
		}
	}

	// Save the byte count and close the scratch file.
	if r.scratch != nil {
		if r.meter != nil {
			if err := writeUsed(r.scratch, r.meter.Used()); err != nil {
				r.le.WithError(err).Warn("save source byte count")
			}
		}
		_ = r.scratch.Close()
	}
}

// indexTarget indexes the target packs its index lacks, such as the last
// pack of a run killed before its rows committed.
func (a *Args) indexTarget(ctx context.Context, r *run) error {
	start := time.Now()
	if err := r.target.BuildIndex(ctx, a.Width, nil); err != nil {
		return errors.Wrap(err, "index target")
	}
	r.le.WithField("took", time.Since(start).String()).Info("target index current")
	return nil
}

// runCopy runs one pass.
func (a *Args) runCopy(*cli.Context) error {
	// Stop on a signal. The walk drains and its last pack still lands.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	r, err := a.open(ctx)
	if err != nil {
		return err
	}
	defer r.close()
	if err := a.indexTarget(ctx, r); err != nil {
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

// openSources builds the source list and the function that closes it: the
// Volume files, the packs, the pulled blocks, then the archive peers.
func (a *Args) openSources(ctx context.Context, r *run) ([]packcopy.Source, func(), error) {
	// Open the Volume files, which answer first.
	sources, closeVolumes, err := a.Volume.open(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Mark the listed blocks in the packs.
	var packs packcopy.Source = r.source
	if a.UnknownRefs != "" {
		list, err := os.Open(a.UnknownRefs)
		if err != nil {
			closeVolumes()
			return nil, nil, err
		}
		defer list.Close()
		if packs, err = packcopy.NewUnknownRefs(r.source, list); err != nil {
			closeVolumes()
			return nil, nil, err
		}
	}
	sources = append(sources, packs)

	// Add the pulled blocks.
	frames, err := packcopy.OpenFrameStore(a.Frames.Value())
	if err != nil {
		closeVolumes()
		return nil, nil, err
	}
	sources = append(sources, frames)

	// Add the archive peers, which answer last.
	archives, closeArchives, err := a.Archive.open(ctx, r.le)
	if err != nil {
		frames.Close()
		closeVolumes()
		return nil, nil, err
	}
	sources = append(sources, archives...)
	return sources, func() {
		closeArchives()
		frames.Close()
		closeVolumes()
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
