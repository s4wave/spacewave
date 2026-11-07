package main

import (
	"context"
	"net/http"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_packsweep_admin "github.com/s4wave/spacewave/db/block/packsweep/admin"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
)

// adminAuthEnv names the variable holding the Authorization header the
// backend's admin service checks.
const adminAuthEnv = "PACK_SWEEP_ADMIN_AUTH" //nolint:gosec // the name of a variable, not a credential.

// adminFlags configures a step that moves packs through the backend.
func (a *Args) adminFlags() []cli.Flag {
	return append(a.scratchFlags(),
		a.adminURLFlag(),
		a.framesFlag(),
		&cli.IntFlag{Name: "batch", Usage: "frontier keys per transaction", Value: packsweep.DefaultWalkBatch, Destination: &a.Batch},
		&cli.StringFlag{Name: "accept-missing", Usage: "file listing missing blocks accepted as lost, one b58 ref a line", Destination: &a.AcceptMissing},
	)
}

// adminURLFlag binds the backend's admin service.
func (a *Args) adminURLFlag() cli.Flag {
	return &cli.StringFlag{Name: "admin-url", Usage: "the backend's pack sweep admin service, a ws or wss URL; the Authorization header comes from " + adminAuthEnv, Destination: &a.AdminURL, Required: true}
}

// trashFlags configures the trash, which reaches the backend and the store.
func (a *Args) trashFlags() []cli.Flag {
	flags := append(a.scratchFlags(), a.adminURLFlag())
	flags = append(flags, a.sftpFlags()...)
	return append(flags, &cli.IntFlag{Name: "moves", Usage: "packs moved to the trash at once, each two renames", Value: packsweep.DefaultMoves, Destination: &a.Moves})
}

// removeFlags configures the removal, which reaches the backend only.
func (a *Args) removeFlags() []cli.Flag {
	return append(a.scratchFlags(), a.adminURLFlag())
}

// waitFlag is how long a pack stays quarantined before it can pass.
func (a *Args) waitFlag() cli.Flag {
	return &cli.DurationFlag{Name: "wait", Usage: "how long a pack stays quarantined before its recheck can pass it", Value: packsweep.DefaultQuarantineWait, Destination: &a.Wait}
}

// dialAdminService reaches the admin service unless a test set another way.
func (a *Args) dialAdminService(ctx context.Context) (*block_packsweep_admin.Client, func(), error) {
	if a.dialAdmin != nil {
		return a.dialAdmin(ctx)
	}
	header := http.Header{}
	if auth := os.Getenv(adminAuthEnv); auth != "" {
		header.Set("Authorization", auth)
	}
	return block_packsweep_admin.DialHTTP(ctx, a.AdminURL, header)
}

// coverOptions opens what a step that moves packs reads: the backend, the
// refs of blocks the catalog lacks, and the accepted holes. The refs come
// from the frame files when named, else from the backend.
func (a *Args) coverOptions(ctx context.Context, store kvtx.Store) (*packsweep.CoverOptions, func(), error) {
	// Read the holes the operator accepted.
	accepted := make(map[string]bool)
	if a.AcceptMissing != "" {
		lines, err := readRefLines(a.AcceptMissing)
		if err != nil {
			return nil, nil, errors.Wrap(err, "--accept-missing")
		}
		for _, line := range lines {
			accepted[line] = true
		}
	}

	// Reach the backend, then the refs.
	client, closeClient, err := a.dialAdminService(ctx)
	if err != nil {
		return nil, nil, err
	}
	refs, closeFrames, err := a.openFrames()
	if err != nil {
		closeClient()
		return nil, nil, err
	}
	if refs == nil {
		refs = client
	}
	opts := &packsweep.CoverOptions{Store: store, Backend: client.Backend(), Refs: refs, Accepted: accepted, Batch: a.Batch}
	return opts, func() { closeFrames(); closeClient() }, nil
}

// runQuarantine quarantines the dead packs through the backend.
func (a *Args) runQuarantine(*cli.Context) error {
	// Open the scratch file and the backend until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()
	opts, closeOpts, err := a.coverOptions(ctx, store)
	if err != nil {
		return err
	}
	defer closeOpts()

	// Quarantine, and print the result.
	res, err := packsweep.Quarantine(ctx, opts)
	if err != nil {
		return err
	}
	return a.print(res.JSON)
}

// runRecheck rechecks the quarantined packs.
func (a *Args) runRecheck(*cli.Context) error {
	// Open the scratch file and the backend until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()
	opts, closeOpts, err := a.coverOptions(ctx, store)
	if err != nil {
		return err
	}
	defer closeOpts()

	// Recheck, and print the result.
	res, err := packsweep.Recheck(ctx, &packsweep.RecheckOptions{CoverOptions: *opts, Wait: a.Wait})
	if err != nil {
		return err
	}
	return a.print(res.JSON)
}

// runTrash moves the passed packs to the trash.
func (a *Args) runTrash(*cli.Context) error {
	return a.runMoves(func(ctx context.Context, opts *packsweep.TrashOptions) (func(*fastjson.Arena) *fastjson.Value, error) {
		res, err := packsweep.Trash(ctx, opts)
		if err != nil {
			return nil, err
		}
		return res.JSON, nil
	})
}

// runRestore moves the missed packs back from the trash.
func (a *Args) runRestore(*cli.Context) error {
	return a.runMoves(func(ctx context.Context, opts *packsweep.TrashOptions) (func(*fastjson.Arena) *fastjson.Value, error) {
		res, err := packsweep.Restore(ctx, opts)
		if err != nil {
			return nil, err
		}
		return res.JSON, nil
	})
}

// runMoves runs a step that moves packs' objects within the store and their
// states through the backend, and prints its result.
func (a *Args) runMoves(step func(context.Context, *packsweep.TrashOptions) (func(*fastjson.Arena) *fastjson.Value, error)) error {
	// Open the scratch file until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()

	// Reach the store, and refuse one the catalog does not describe: its
	// files are not the packs the sweep judged.
	objects, root, closeStore, err := a.dialStore()
	if err != nil {
		return err
	}
	defer closeStore()
	mover, ok := objects.(block_store_s3.ObjectMover)
	if !ok {
		return errors.New("the store cannot move its objects")
	}
	if err := checkStore(ctx, db, a.storeBinding(root)); err != nil {
		return err
	}

	// Reach the backend, run the step, and print the result.
	client, closeClient, err := a.dialAdminService(ctx)
	if err != nil {
		return err
	}
	defer closeClient()
	res, err := step(ctx, &packsweep.TrashOptions{
		Store:   store,
		Backend: client.Backend(),
		Mover:   mover,
		Bucket:  root,
		Prefix:  a.Prefix,
		Moves:   a.Moves,
	})
	if err != nil {
		return err
	}
	return a.print(res)
}

// runRemove has the backend remove the moved packs.
func (a *Args) runRemove(*cli.Context) error {
	// Open the scratch file and the backend until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()
	client, closeClient, err := a.dialAdminService(ctx)
	if err != nil {
		return err
	}
	defer closeClient()

	// Remove, and print the result.
	res, err := packsweep.Remove(ctx, &packsweep.RemoveOptions{Store: store, Backend: client.Backend()})
	if err != nil {
		return err
	}
	return a.print(res.JSON)
}

// recordCounts counts the pack records by stage.
func recordCounts(ctx context.Context, store kvtx.Store) (map[string]uint64, error) {
	counts := make(map[string]uint64)
	err := packsweep.PackRecords(ctx, store, func(p *packsweep.PackRecord) error {
		counts[p.Stage.String()]++
		return nil
	})
	return counts, err
}
