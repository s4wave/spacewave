package main

import (
	"context"
	"net/http"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_packsweep_admin "github.com/s4wave/spacewave/db/block/packsweep/admin"
	"github.com/s4wave/spacewave/db/kvtx"
)

// adminAuthEnv names the variable holding the Authorization header the
// backend's admin service checks.
const adminAuthEnv = "PACK_SWEEP_ADMIN_AUTH" //nolint:gosec // the name of a variable, not a credential.

// adminFlags configures a step that moves packs through the backend.
func (a *Args) adminFlags() []cli.Flag {
	return append(a.scratchFlags(),
		&cli.StringFlag{Name: "admin-url", Usage: "the backend's pack sweep admin service, a ws or wss URL; the Authorization header comes from " + adminAuthEnv, Destination: &a.AdminURL, Required: true},
		a.framesFlag(),
		&cli.IntFlag{Name: "batch", Usage: "frontier keys per transaction", Value: packsweep.DefaultWalkBatch, Destination: &a.Batch},
		&cli.StringFlag{Name: "accept-missing", Usage: "file listing missing blocks accepted as lost, one b58 ref a line", Destination: &a.AcceptMissing},
	)
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

// recordCounts counts the pack records by stage.
func recordCounts(ctx context.Context, store kvtx.Store) (map[string]uint64, error) {
	counts := make(map[string]uint64)
	err := packsweep.PackRecords(ctx, store, func(p *packsweep.PackRecord) error {
		counts[p.Stage.String()]++
		return nil
	})
	return counts, err
}
