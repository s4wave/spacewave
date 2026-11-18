package main

import (
	"context"
	"encoding/hex"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_packsweep_admin "github.com/s4wave/spacewave/db/block/packsweep/admin"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile/writer"
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

// trashFlags configures the trash, which covers the cycle as the quarantine
// does and reaches the store.
func (a *Args) trashFlags() []cli.Flag {
	flags := append(a.adminFlags(), a.dialFlags()...)
	return append(flags,
		&cli.IntFlag{Name: "moves", Usage: "packs moved to the trash at once, each two renames", Value: packsweep.DefaultMoves, Destination: &a.Moves},
		&cli.StringFlag{Name: "trash-method", Usage: "how the trash sets a pack aside: move copies it to the trash and deletes every version of the original; hide puts a delete marker over it, on a bucket that keeps old versions, and deletes nothing", Value: packsweep.TrashMove.String(), Destination: &a.TrashMethod},
		&cli.BoolFlag{Name: noLifecycleRuleFlag, Usage: "state that the bucket has no lifecycle rule that hides or deletes versions under --prefix, which a hide needs on a store that is not B2; on B2 the trash reads the rules itself and ignores this", Destination: &a.NoLifecycleRule},
	)
}

// restoreFlags configures the restore: the trash's, and the packs to bring
// back besides those reads missed.
func (a *Args) restoreFlags() []cli.Flag {
	return append(a.trashFlags(), &cli.StringSliceFlag{Name: "pack", Usage: "a trashing or removed pack to restore besides those reads missed; repeat for each", Destination: &a.Packs})
}

// compactFlags configures the compaction, which covers the cycle as the
// quarantine does and reaches the store.
func (a *Args) compactFlags() []cli.Flag {
	flags := append(a.adminFlags(), a.dialFlags()...)
	return append(flags, &cli.Int64Flag{Name: "max-pack-bytes", Usage: "most bytes of a pack the compaction writes", Value: writer.DefaultMaxPackBytes, Destination: &a.MaxPackBytes})
}

// removeFlags configures the removal, which covers the cycle as the
// quarantine does.
func (a *Args) removeFlags() []cli.Flag {
	return a.adminFlags()
}

// purgeFlags configures the purge, which reaches the store, and the backend
// to check each pack against.
func (a *Args) purgeFlags() []cli.Flag {
	flags := append(a.scratchFlags(), a.adminURLFlag())
	flags = append(flags, a.dialFlags()...)
	return append(flags,
		&cli.DurationFlag{Name: "after", Usage: "how long since its removal a pack stays in the trash before the purge deletes it", Value: packsweep.DefaultPurgeAfter, Destination: &a.After},
		&cli.StringSliceFlag{Name: "pack", Usage: "a removed pack to purge; repeat for each; none purges every removed pack past --after", Destination: &a.Packs},
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
	return dialAdminURL(ctx, a.AdminURL)
}

// dialAdminURL reaches the admin service at url, with the Authorization
// header from adminAuthEnv.
func dialAdminURL(ctx context.Context, url string) (*block_packsweep_admin.Client, func(), error) {
	header := http.Header{}
	if auth := os.Getenv(adminAuthEnv); auth != "" {
		header.Set("Authorization", auth)
	}
	return block_packsweep_admin.DialHTTP(ctx, url, header)
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

// runCompact writes the live blocks of the low packs into new packs.
func (a *Args) runCompact(*cli.Context) error {
	// Open the scratch file until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()

	// Reach the store, refusing one the catalog does not describe.
	objects, root, closeStore, err := a.dialStore()
	if err != nil {
		return err
	}
	defer closeStore()
	if err := checkStore(ctx, db, a.storeBinding(root)); err != nil {
		return err
	}

	// Reach the backend and the refs.
	opts, closeOpts, err := a.coverOptions(ctx, store)
	if err != nil {
		return err
	}
	defer closeOpts()

	// Compact, and print the result.
	res, err := packsweep.Compact(ctx, &packsweep.CompactOptions{CoverOptions: *opts, Objects: objects, Bucket: root, Prefix: a.Prefix, MaxPackBytes: a.MaxPackBytes})
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

// runRestore moves the missed and the named packs back from the trash.
func (a *Args) runRestore(*cli.Context) error {
	return a.runMoves(func(ctx context.Context, opts *packsweep.TrashOptions) (func(*fastjson.Arena) *fastjson.Value, error) {
		res, err := packsweep.Restore(ctx, &packsweep.RestoreOptions{TrashOptions: *opts, Packs: a.Packs.Value()})
		if err != nil {
			return nil, err
		}
		return res.JSON, nil
	})
}

// forgetFlags are the flags of the forget step.
func (a *Args) forgetFlags() []cli.Flag {
	return []cli.Flag{
		a.adminURLFlag(),
		&cli.StringSliceFlag{Name: "pack", Usage: "a registered pack whose recorded read to clear; repeat for each", Destination: &a.Packs, Required: true},
	}
}

// runForget clears the read recorded on the named packs, and prints their
// states after.
func (a *Args) runForget(*cli.Context) error {
	// Reach the backend until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	client, closeClient, err := a.dialAdminService(ctx)
	if err != nil {
		return err
	}
	defer closeClient()

	// Forget a batch at a time, and print the states.
	var holds []packsweep.PackHold
	for batch := range slices.Chunk(a.Packs.Value(), packsweep.MaxBackendPacks) {
		got, err := client.Backend().Forget(ctx, batch)
		if err != nil {
			return err
		}
		holds = append(holds, got...)
	}
	return a.print(func(arena *fastjson.Arena) *fastjson.Value {
		arr := arena.NewArray()
		for i, h := range holds {
			o := arena.NewObject()
			o.Set("pack", arena.NewString(h.ID))
			o.Set("state", arena.NewNumberInt(int(h.State)))
			o.Set("key", arena.NewString(hex.EncodeToString(h.Key)))
			arr.SetArrayItem(i, o)
		}
		return arr
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

	// Refuse a hide on a store that cannot hide, and check the store.
	method, err := packsweep.ParseTrashMethod(a.TrashMethod)
	if err != nil {
		return err
	}
	hider, _ := objects.(block_store_s3.ObjectHider)
	if method == packsweep.TrashHide && hider == nil {
		return errors.New("the store cannot hide its objects: --trash-method hide needs a versioned S3 bucket")
	}
	if err := checkStore(ctx, db, a.storeBinding(root)); err != nil {
		return err
	}

	// A hide needs the finding that no lifecycle rule deletes what it keeps.
	var clearance block_store_s3.HideClearance
	if method == packsweep.TrashHide {
		if clearance, err = a.hideClearance(ctx, root); err != nil {
			return err
		}
	}

	// Reach the backend and the refs, run the step, and print the result.
	opts, closeOpts, err := a.coverOptions(ctx, store)
	if err != nil {
		return err
	}
	defer closeOpts()
	res, err := step(ctx, &packsweep.TrashOptions{
		CoverOptions: *opts,
		Mover:        mover,
		Hider:        hider,
		Clearance:    clearance,
		Method:       method,
		Bucket:       root,
		Prefix:       a.Prefix,
		Moves:        a.Moves,
	})
	if err != nil {
		return err
	}
	return a.print(res)
}

// noLifecycleRuleFlag is the operator's statement that the bucket has no
// lifecycle rule that hides or deletes versions under the prefix.
const noLifecycleRuleFlag = "bucket-has-no-lifecycle-rule"

// hideClearance finds that no lifecycle rule of bucket hides or deletes the
// versions a hide keeps under the prefix. On B2 it reads the rules through
// B2's native API with the store's own key, whatever the flag says; on any
// other store it takes the operator's --bucket-has-no-lifecycle-rule.
func (a *Args) hideClearance(ctx context.Context, bucket string) (block_store_s3.HideClearance, error) {
	// Read B2's rules.
	api := a.b2API
	if api == "" && isB2Endpoint(a.S3Endpoint) {
		api = block_store_s3.B2APIURL
	}
	if api != "" {
		return block_store_s3.ReadB2HideClearance(ctx, block_store_s3.B2Account{
			APIURL: api,
			KeyID:  os.Getenv(s3KeyIDEnv),
			Key:    os.Getenv(s3SecretEnv),
		}, bucket, a.Prefix)
	}

	// Elsewhere take the operator's statement.
	if !a.NoLifecycleRule {
		return block_store_s3.HideClearance{}, errors.Errorf("--trash-method hide needs to know that no lifecycle rule deletes hidden versions: on a store that is not B2, check the bucket and pass --%s", noLifecycleRuleFlag)
	}
	return block_store_s3.StatedHideClearance(bucket, a.Prefix), nil
}

// isB2Endpoint reports whether endpoint, a host or URL, is a Backblaze B2 S3
// endpoint, whose lifecycle rules B2's native API reads.
func isB2Endpoint(endpoint string) bool {
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	host, _, _ = strings.Cut(host, "/")
	host, _, _ = strings.Cut(host, ":")
	return strings.HasSuffix(strings.ToLower(host), ".backblazeb2.com")
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
	opts, closeOpts, err := a.coverOptions(ctx, store)
	if err != nil {
		return err
	}
	defer closeOpts()

	// Remove, and print the result.
	res, err := packsweep.Remove(ctx, &packsweep.RemoveOptions{CoverOptions: *opts})
	if err != nil {
		return err
	}
	return a.print(res.JSON)
}

// runPurge deletes every version of each removed pack's objects once it has
// been removed for --after, printing a line for each version deleted.
func (a *Args) runPurge(*cli.Context) error {
	// Open the scratch file until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()

	// Reach the store, and refuse one the catalog does not describe or that
	// cannot delete a version by its id.
	objects, root, closeStore, err := a.dialStore()
	if err != nil {
		return err
	}
	defer closeStore()
	purger, ok := objects.(block_store_s3.ObjectPurger)
	if !ok {
		return errors.New("the store cannot delete an object's versions by id")
	}
	if err := checkStore(ctx, db, a.storeBinding(root)); err != nil {
		return err
	}

	// Reach the backend, which the purge checks each pack against.
	client, closeClient, err := a.dialAdminService(ctx)
	if err != nil {
		return err
	}
	defer closeClient()
	checkRoots := func(ctx context.Context) error {
		_, _, err := client.WalkRoots(ctx)
		return err
	}

	// Purge, printing each deleted version, then the result.
	var printErr error
	res, err := packsweep.Purge(ctx, &packsweep.PurgeOptions{
		Store:      store,
		Purger:     purger,
		Backend:    client.Backend(),
		CheckRoots: checkRoots,
		Bucket:     root,
		Prefix:     a.Prefix,
		After:      a.After,
		Packs:      a.Packs.Value(),
		Deleted: func(key, version string) {
			if printErr == nil {
				printErr = a.print(func(ar *fastjson.Arena) *fastjson.Value {
					v := ar.NewObject()
					v.Set("deleted", ar.NewString(key))
					v.Set("version", ar.NewString(version))
					return v
				})
			}
		},
	})
	if err != nil {
		return err
	}
	if printErr != nil {
		return printErr
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
