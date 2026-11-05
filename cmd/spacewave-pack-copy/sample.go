package main

import (
	"cmp"
	"context"
	"math/rand/v2"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/packcopy"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_sftp "github.com/s4wave/spacewave/db/block/store/sftp"
	"github.com/s4wave/spacewave/db/packfile"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// sampleArgs configures the sample command.
type sampleArgs struct {
	// count is the reads to time.
	count int
	// bytes is the length of each read, about one block.
	bytes int
	// reads is the reads kept in flight.
	reads int
}

// sampleFlags binds the flags of the sample command: the source's, and the
// sample's own.
func (a *Args) sampleFlags(s *sampleArgs) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "store-id", Usage: "block store id whose pack index the index file holds", Destination: &a.StoreID, Required: true},
		&cli.StringFlag{Name: "source-index", Usage: "the source's pack index file, opened read-only beside its writer", Destination: &a.SourceIndex, Required: true},
		&cli.StringFlag{Name: "index-table", Usage: "table of the pack index file", Value: defaultIndexTable, Destination: &a.IndexTable},
		&cli.StringFlag{Name: "sftp-addr", Usage: "source server's host:port", Destination: &a.SFTPAddr, Required: true},
		&cli.StringFlag{Name: "sftp-user", Usage: "source login; the password comes from " + sftpPasswordEnv, Destination: &a.SFTPUser, Required: true},
		&cli.StringFlag{Name: "sftp-host-key", Usage: "source server's host key line or SHA256 fingerprint", Destination: &a.SFTPHostKey, Required: true},
		&cli.IntFlag{Name: "sftp-conns", Usage: "most SSH connections to the source at once, each carrying many reads", Value: 2, Destination: &a.SFTPConns},
		&cli.StringFlag{Name: "sftp-root", Usage: "source directory holding the source prefix", Destination: &a.SFTPRoot, Required: true},
		&cli.StringFlag{Name: "source-prefix", Usage: "key prefix of the source's packs", Destination: &a.SourcePrefix},
		&cli.IntFlag{Name: "count", Usage: "reads to time", Value: 2000, Destination: &s.count},
		&cli.IntFlag{Name: "bytes", Usage: "length of each read", Value: 4096, Destination: &s.bytes},
		&cli.IntFlag{Name: "reads", Usage: "reads kept in flight", Value: packcopy.DefaultReads, Destination: &s.reads},
	}
}

// runSample times reads of one block's length at random places in random
// packs the source's index lists, as a copy's block reads fall, and logs
// their rate and latency. It writes nothing.
func (a *Args) runSample(s *sampleArgs) error {
	// Stop on a signal.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	le := logrus.NewEntry(logrus.StandardLogger())

	// Pick the packs to read from the index's entry rows.
	packs, err := a.samplePacks(ctx, s.count)
	if err != nil {
		return err
	}
	if len(packs) == 0 {
		return errors.New("the index lists no packs")
	}

	// Reach the source within its connection cap.
	client, err := block_store_sftp.NewClient(block_store_sftp.Config{
		Addr:     a.SFTPAddr,
		User:     a.SFTPUser,
		Password: os.Getenv(sftpPasswordEnv),
		HostKey:  a.SFTPHostKey,
		Conns:    a.SFTPConns,
	})
	if err != nil {
		return err
	}
	defer client.Close()

	// Time one read in each pack, s.reads at once.
	var mu sync.Mutex
	took := make([]time.Duration, 0, len(packs))
	var total int64
	start := time.Now()
	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(max(s.reads, 1))
	for _, p := range packs {
		eg.Go(func() error {
			// Read at a random place in the pack.
			off := rand.Int64N(max(p.size-int64(s.bytes), 1)) //nolint:gosec // a sample place, not a secret.
			t := time.Now()
			data, err := client.GetObjectRange(ectx, a.SFTPRoot, a.SourcePrefix+"packs/"+p.id, off, s.bytes)
			if err != nil {
				return errors.Wrap(err, p.id)
			}

			// Record its time and size.
			mu.Lock()
			took = append(took, time.Since(t))
			total += int64(len(data))
			mu.Unlock()
			return nil
		})
	}
	err = eg.Wait()

	// Report what finished.
	elapsed := time.Since(start)
	slices.Sort(took)
	quantile := func(q float64) time.Duration {
		if len(took) == 0 {
			return 0
		}
		return took[min(int(q*float64(len(took))), len(took)-1)]
	}
	le.WithFields(logrus.Fields{
		"reads":     len(took),
		"bytes":     total,
		"took":      elapsed.Round(time.Millisecond).String(),
		"reads-sec": float64(len(took)) / elapsed.Seconds(),
		"p50":       quantile(0.5).String(),
		"p90":       quantile(0.9).String(),
		"p99":       quantile(0.99).String(),
		"conns":     a.SFTPConns,
		"in-flight": s.reads,
	}).Info("sample finished")
	return err
}

// samplePack is one pack a sample reads.
type samplePack struct {
	// id is the pack's id.
	id string
	// size is the pack's size in bytes.
	size int64
}

// samplePacks returns up to n packs drawn evenly from the source index's
// entry rows, in one scan of the rows through a read-only open.
func (a *Args) samplePacks(ctx context.Context, n int) ([]samplePack, error) {
	// Open the index beside its writer.
	index, err := store_kvtx_sqlite.OpenWithPragmas(ctx, a.SourceIndex, cmp.Or(a.IndexTable, defaultIndexTable), store_kvtx_sqlite.Pragmas{ReadOnly: true})
	if err != nil {
		return nil, errors.Wrap(err, "open --source-index")
	}
	defer index.Close()
	tx, err := index.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Keep a uniform sample of the entry rows.
	prefix := append(block_store_s3.PackIndexPrefix(a.StoreID), "e/"...)
	var packs []samplePack
	seen := 0
	it := tx.Iterate(ctx, prefix, false, false)
	defer it.Close()
	for it.Next() {
		value, err := it.Value()
		if err != nil {
			return nil, err
		}
		entry := &packfile.PackfileEntry{}
		if err := entry.UnmarshalVT(value); err != nil || entry.GetSizeBytes() == 0 {
			continue
		}
		p := samplePack{id: entry.GetId(), size: int64(entry.GetSizeBytes())} //nolint:gosec // a packfile is far smaller than 2^63 bytes.
		seen++
		if len(packs) < n {
			packs = append(packs, p)
		} else if i := rand.IntN(seen); i < n { //nolint:gosec // a sample draw, not a secret.
			packs[i] = p
		}
	}
	return packs, it.Err()
}
