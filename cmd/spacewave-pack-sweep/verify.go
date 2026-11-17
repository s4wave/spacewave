package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_packsweep_admin "github.com/s4wave/spacewave/db/block/packsweep/admin"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/sirupsen/logrus"
)

// verifyFlags binds the flags of the verify: the store's, without the id of a
// pack index it does not read, and the roots, named or from the backend.
func (a *Args) verifyFlags() []cli.Flag {
	flags := slices.DeleteFunc(a.dialFlags(), func(f cli.Flag) bool {
		return slices.Contains(f.Names(), "store-id")
	})
	return append(flags,
		&cli.StringSliceFlag{Name: "root", Usage: "root block, as a b58 ref; repeat for each", Destination: &a.Roots},
		&cli.StringFlag{Name: "roots-file", Usage: "file listing more roots, one b58 ref a line", Destination: &a.RootsFile},
		&cli.StringFlag{Name: "admin-url", Usage: "read every named root and pin from the backend's pack sweep admin service, a ws or wss URL; the Authorization header comes from " + adminAuthEnv, Destination: &a.AdminURL},
		&cli.IntFlag{Name: "batch", Usage: "frontier keys per transaction", Value: packsweep.DefaultVerifyBatch, Destination: &a.Batch},
		&cli.IntFlag{Name: "reads", Usage: "blocks read at once", Value: packsweep.DefaultVerifyReads, Destination: &a.Reads},
		&cli.IntFlag{Name: "max-bad", Usage: "bad blocks listed; every one is counted", Value: packsweep.DefaultVerifyMaxBad, Destination: &a.MaxBad},
	)
}

// runVerify reads every block the roots reach from the bucket alone, through
// a fresh pack store with no local index, and checks each against its hash.
// It prints the counts per root and the first bad blocks, and fails when any
// block is missing, unreadable, corrupt or without its refs.
func (a *Args) runVerify(*cli.Context) error {
	// Run until a signal stops the verify.
	ctx, cancel := signalContext()
	defer cancel()
	le := logrus.NewEntry(logrus.StandardLogger())

	// Read the roots, then reach the bucket.
	roots, err := a.verifyRoots(ctx)
	if err != nil {
		return err
	}
	objects, bucket, closeStore, err := a.dialStore()
	if err != nil {
		return err
	}
	defer closeStore()

	// Open the live packs, and the trashed ones to name a block only they
	// hold, both read-only.
	store := block_store_s3.NewPackStore(le, readOnly{objects}, bucket, a.Prefix, &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer store.Close()
	trash := block_store_s3.NewPackStore(le.WithField("packs", "trash"), readOnly{objects}, bucket, block_store_s3.TrashPrefix(a.Prefix), &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer trash.Close()

	// Keep the blocks read and the frontier in a scratch file of the
	// verify's own, removed after.
	dir, err := os.MkdirTemp("", "pack-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	db, err := s4db.Open(filepath.Join(dir, "verify"), s4db.Options{})
	if err != nil {
		return err
	}
	defer db.Close()

	// Verify, logging the totals at most every few seconds.
	start, logged := time.Now(), time.Now()
	res, err := packsweep.Verify(ctx, &packsweep.VerifyOptions{
		Store:  db,
		Source: store,
		Trash:  trash,
		Roots:  roots,
		Batch:  a.Batch,
		Reads:  a.Reads,
		MaxBad: a.MaxBad,
		Progress: func(r *packsweep.VerifyResult) {
			if time.Since(logged) < 5*time.Second {
				return
			}
			logged = time.Now()
			le.WithFields(logrus.Fields{"blocks": r.Blocks, "bytes": r.Bytes, "bad": r.Bad, "roots": len(r.Roots)}).Info("verify batch")
		},
	})
	if err != nil {
		return err
	}
	le.WithField("took", time.Since(start).String()).Info("verify complete")
	if err := a.print(res.JSON); err != nil {
		return err
	}
	if res.Bad != 0 {
		return errors.Errorf("%d blocks missing, trashed, unreadable, corrupt or without their refs", res.Bad)
	}
	return nil
}

// verifyRoots returns the roots named on the command line, each named by its
// ref, then every named root and pin the backend lists.
func (a *Args) verifyRoots(ctx context.Context) ([]block.NamedRoot, error) {
	// Read the named roots.
	values := a.Roots.Value()
	if a.RootsFile != "" {
		lines, err := readRefLines(a.RootsFile)
		if err != nil {
			return nil, errors.Wrap(err, "--roots-file")
		}
		values = append(values, lines...)
	}
	var roots []block.NamedRoot
	for _, value := range values {
		ref, err := block.UnmarshalBlockRefB58(value)
		if err != nil {
			return nil, errors.Wrap(err, "parse root "+value)
		}
		roots = append(roots, block.NamedRoot{Name: value, Ref: ref})
	}

	// Add the backend's roots and pins.
	if a.AdminURL != "" || a.dialAdmin != nil {
		client, closeClient, err := a.dialAdminService(ctx)
		if err != nil {
			return nil, err
		}
		defer closeClient()
		resp, err := client.Roots(ctx, &block_packsweep_admin.RootsRequest{})
		if err != nil {
			return nil, errors.Wrap(err, "read the backend's roots")
		}
		for _, root := range resp.GetRoots() {
			roots = append(roots, block.NamedRoot{Name: root.GetName(), Ref: root.GetRef()})
		}
		for _, pin := range resp.GetPins() {
			roots = append(roots, block.NamedRoot{Name: "pin", Ref: pin})
		}
	}
	if len(roots) == 0 {
		return nil, errors.New("name the roots with --root, --roots-file or --admin-url")
	}
	return roots, nil
}

// readOnly is a store that refuses every write and delete, so a verify only
// ever reads the bucket.
type readOnly struct {
	block_store_s3.ObjectStore
}

// errReadOnly refuses a write.
var errReadOnly = errors.New("the verify only reads the store")

// PutObject refuses the write.
func (readOnly) PutObject(context.Context, string, string, []byte, string) error {
	return errReadOnly
}

// DeleteObject refuses the delete.
func (readOnly) DeleteObject(context.Context, string, string) error {
	return errReadOnly
}
