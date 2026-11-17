//go:build !tinygo

package packsweep

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
)

// DefaultPurgeAfter is how long a removed pack stays in the trash before a
// purge may delete it when the options set no other wait.
const DefaultPurgeAfter = 30 * 24 * time.Hour

// PurgeOptions configures Purge.
type PurgeOptions struct {
	// Store is the scratch store.
	Store kvtx.Store
	// Purger deletes every version of an object by its version id.
	Purger block_store_s3.ObjectPurger
	// Backend answers, just before a pack's versions go, whether the store's
	// index still lacks the pack.
	Backend Backend
	// CheckRoots lists the backend's roots, every restore point's included,
	// and returns the listing's error: it fails while a restore point names
	// packs by a pack index. Purge calls it before the first deletion and
	// again whenever the backend's log epoch changes, since a restore point
	// of that kind appears only after a restart onto another store.
	CheckRoots func(ctx context.Context) error
	// Bucket is the store's bucket.
	Bucket string
	// Prefix starts the store's object keys.
	Prefix string
	// After is how long since its removal a pack waits before the purge;
	// zero uses DefaultPurgeAfter.
	After time.Duration
	// Packs limits the purge to these removed packs; empty purges every one
	// past the wait.
	Packs []string
	// Now returns the purge time; nil uses time.Now.
	Now func() time.Time
	// Deleted sees each version the purge deleted, after the store confirmed
	// it, so the operator keeps a line for every one.
	Deleted func(key, version string)
}

// PurgeResult reports a purge.
type PurgeResult struct {
	// Purged is the packs whose every object version this run deleted.
	Purged uint64
	// Versions is the object versions this run deleted.
	Versions uint64
	// Waiting is the removed packs not yet past the wait.
	Waiting uint64
}

// JSON describes the purge.
func (r *PurgeResult) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"purged", jsonUint(a, r.Purged)},
		jsonField{"versions", jsonUint(a, r.Versions)},
		jsonField{"waiting", jsonUint(a, r.Waiting)},
	)
}

// Purge deletes, by version id, every version of each removed pack's objects
// once the pack has been removed for the wait: its entry and file in the
// trash, and any version left under its original keys, such as one a move's
// delete hid instead of deleting. A bucket that keeps old versions bills a
// hidden one until its id is deleted, and a plain delete only hides the
// current one, so the purger never falls back to it. The pack's record turns
// purged only after every key's versions are gone, so a run killed anywhere
// purges the rest when repeated, and a purged pack can no longer be restored.
//
// Immediately before deleting a pack's versions it checks again that the
// pack's record is still removed, that the backend's index still lacks it,
// and that the roots listing still succeeds, so no restore point names it;
// the first check that fails stops the run with that pack and the rest kept.
// That costs a scratch read and a status call per pack, and a roots listing
// per run and per backend epoch.
func Purge(ctx context.Context, opts *PurgeOptions) (*PurgeResult, error) {
	// Read the pack records, refusing a store that cannot purge or check.
	if opts.Purger == nil {
		return nil, errors.New("purge: the store deletes no version by id")
	}
	if opts.Backend == nil || opts.CheckRoots == nil {
		return nil, errors.New("purge: no backend to check each pack against")
	}
	records, err := readPackRecords(ctx, opts.Store)
	if err != nil {
		return nil, err
	}

	// Find the cutoff: packs removed before it have waited long enough.
	after := opts.After
	if after <= 0 {
		after = DefaultPurgeAfter
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	cutoff := now().Add(-after)

	// Choose the removed packs past the wait.
	res := &PurgeResult{}
	var todo []*PackRecord
	for _, p := range records {
		switch {
		case len(opts.Packs) != 0 && !slices.Contains(opts.Packs, p.ID):
		case p.Stage != StageRemoved:
		case p.Removed.IsZero() || p.Removed.After(cutoff):
			res.Waiting++
		default:
			todo = append(todo, p)
		}
	}

	// Check each pack again, purge its keys, then record it purged.
	check := &purgeCheck{opts: opts}
	for _, p := range todo {
		if err := check.pack(ctx, p.ID); err != nil {
			return res, err
		}
		for _, key := range purgeKeys(opts.Prefix, p.ID) {
			err := opts.Purger.PurgeObject(ctx, opts.Bucket, key, func(version string) {
				res.Versions++
				if opts.Deleted != nil {
					opts.Deleted(key, version)
				}
			})
			if err != nil {
				return res, errors.Wrapf(err, "purge %s", key)
			}
		}
		p.Stage = StagePurged
		if err := writePackRecords(ctx, opts.Store, []*PackRecord{p}, nil); err != nil {
			return res, err
		}
		res.Purged++
	}
	return res, nil
}

// purgeCheck rechecks each pack just before a purge deletes its versions,
// listing the roots again when the backend's epoch changes.
type purgeCheck struct {
	opts *PurgeOptions
	// listed is set once the roots listing succeeded on epoch.
	listed bool
	epoch  string
}

// pack checks that pack id's record is still removed, that the backend still
// lacks it, and that the roots listing succeeds on the backend's epoch.
func (c *purgeCheck) pack(ctx context.Context, id string) error {
	// Read the record again.
	p, err := readPackRecord(ctx, c.opts.Store, id)
	if err != nil {
		return err
	}
	if p == nil || p.Stage != StageRemoved {
		return errors.Errorf("purge pack %s: its record left the removed stage", id)
	}

	// Ask the backend whether it holds the pack.
	st, err := c.opts.Backend.Status(ctx, []string{id})
	if err != nil {
		return errors.Wrap(err, "status")
	}
	if len(st.Packs) != 1 || st.Packs[0].ID != id || !st.Packs[0].Absent {
		return errors.Errorf("purge pack %s: the backend holds it", id)
	}

	// List the roots on a new epoch.
	if c.listed && st.LogEpoch == c.epoch {
		return nil
	}
	if err := c.opts.CheckRoots(ctx); err != nil {
		return errors.Wrapf(err, "purge pack %s: list the roots and restore points", id)
	}
	c.listed, c.epoch = true, st.LogEpoch
	return nil
}

// purgeKeys are the object keys of pack id a purge deletes every version of:
// the trash's file and entry, then the original file and entry.
func purgeKeys(prefix, id string) []string {
	return []string{
		block_store_s3.TrashPackKey(prefix, id),
		block_store_s3.TrashEntryKey(prefix, id),
		block_store_s3.PackObjectKey(prefix, id),
		block_store_s3.EntryObjectKey(prefix, id),
	}
}
