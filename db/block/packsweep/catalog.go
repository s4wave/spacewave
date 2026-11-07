package packsweep

import (
	"bytes"
	"context"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"golang.org/x/sync/errgroup"
)

// DefaultPackReads is the packs a snapshot reads at once when the options set
// none. Each holds up to one pack's bytes in memory.
const DefaultPackReads = 4

// snapshotBatch bounds the snapshot rows one transaction writes.
const snapshotBatch = 4096

// commitKeys is the keys past which the catalog stops adding waiting packs to
// a transaction.
const commitKeys = 16384

// readyPacks is the parsed packs waiting to commit. A parsed pack holds its
// keys and refs, not its bytes, so many small packs wait in little memory.
const readyPacks = 64

// PackSource lists the packs a store has registered and reads them.
// PackStore implements it.
type PackSource interface {
	// Packs returns every registered pack.
	Packs(ctx context.Context) ([]*packfile.PackfileEntry, error)
	// ReadPackFile reads the whole of pack id, which is size bytes.
	ReadPackFile(ctx context.Context, id string, size uint64) ([]byte, error)
}

// SnapshotOptions configures Snapshot.
type SnapshotOptions struct {
	// Store is the scratch store. The sweep owns every key in it, so pass a
	// prefixed view.
	Store kvtx.Store
	// Packs lists and reads the packs.
	Packs PackSource
	// Reads is the packs read at once. Zero uses DefaultPackReads.
	Reads int
	// Now returns the time S; nil uses time.Now.
	Now func() time.Time
	// Progress observes each catalogued pack, nil for none.
	Progress func(SnapshotProgress)
}

// SnapshotProgress reports a snapshot.
type SnapshotProgress struct {
	// Packs is the packs registered at S.
	Packs int
	// Catalogued is the packs this run read into the catalog.
	Catalogued int
	// Skipped is the packs already in the catalog.
	Skipped int
	// Bytes is the bytes this run read.
	Bytes uint64
}

// Snapshot takes the pack list at S, unless this cycle already holds one, and
// reads every listed pack the catalog lacks into the catalog. A run killed at
// any point resumes from the packs whose rows committed.
func Snapshot(ctx context.Context, opts *SnapshotOptions) (*SnapshotProgress, error) {
	// Take the pack list once per cycle.
	head, err := ReadHead(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	if head.SnapshotAt.IsZero() {
		if head, err = takeSnapshot(ctx, opts); err != nil {
			return nil, err
		}
	}

	// List the snapshot packs the catalog lacks.
	todo, skipped, err := uncatalogued(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	progress := &SnapshotProgress{Packs: head.SnapshotPacks, Skipped: skipped}
	if err := catalogPacks(ctx, opts, todo, progress); err != nil {
		return progress, err
	}

	// Mark the catalog complete for the cycle.
	_, err = updateHead(ctx, opts.Store, func(h *Head) error {
		h.Catalogued = true
		return nil
	})
	return progress, err
}

// takeSnapshot writes the registered packs as the cycle's snapshot, then
// records S in the header. A run killed before the header commits lists again
// and writes the same rows over the partial ones.
func takeSnapshot(ctx context.Context, opts *SnapshotOptions) (*Head, error) {
	// List the packs, then note S: a pack registered after the listing is
	// not in it, and no pack in it was registered after S.
	entries, err := opts.Packs.Packs(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list packs")
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	at := now().UTC()

	// Drop a partial list from an earlier run, then write this one.
	if err := deletePrefix(ctx, opts.Store, []byte(snapshotPrefix)); err != nil {
		return nil, err
	}
	for start := 0; start < len(entries); start += snapshotBatch {
		end := min(start+snapshotBatch, len(entries))
		tx, err := opts.Store.NewTransaction(ctx, true)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries[start:end] {
			row := snapshotPack{size: entry.GetSizeBytes(), created: createdUnix(entry)}
			if err := tx.Set(ctx, []byte(snapshotPrefix+entry.GetId()), row.marshal()); err != nil {
				tx.Discard()
				return nil, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Discard()
			return nil, err
		}
		tx.Discard()
	}
	return updateHead(ctx, opts.Store, func(h *Head) error {
		h.SnapshotAt = at
		h.SnapshotPacks = len(entries)
		return nil
	})
}

// createdUnix returns the creation time of an entry in Unix seconds, zero
// when it has none.
func createdUnix(entry *packfile.PackfileEntry) int64 {
	if entry.GetCreatedAt() == nil {
		return 0
	}
	return entry.GetCreatedAt().AsTime().Unix()
}

// pendingPack is a snapshot pack the catalog lacks.
type pendingPack struct {
	// id is the pack.
	id string
	// row is its snapshot row.
	row snapshotPack
}

// uncatalogued lists the snapshot packs without a catalog row, and counts the
// rest.
func uncatalogued(ctx context.Context, store kvtx.Store) ([]pendingPack, int, error) {
	// Open a read.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Discard()

	// List the snapshot packs.
	rows, err := scanSnapshot(ctx, tx)
	if err != nil {
		return nil, 0, err
	}

	// Keep those the catalog lacks, counting the rest.
	var todo []pendingPack
	var skipped int
	for _, p := range rows {
		found, err := tx.Exists(ctx, []byte(catalogPackPrefix+p.id))
		if err != nil {
			return nil, 0, err
		}
		if found {
			skipped++
			continue
		}
		todo = append(todo, p)
	}
	return todo, skipped, nil
}

// readPack is one pack read and parsed, ready to commit.
type readPack struct {
	// id is the pack.
	id string
	// row is its catalog row.
	row catalogPack
	// keys are its members in key order.
	keys []packKey
}

// packKey is one key of a read pack.
type packKey struct {
	// tail is the key's tail.
	tail []byte
	// member is its membership row.
	member member
	// state is the refs state of this copy.
	state byte
	// refs is the copy's encoded refs row, nil unless state is refsKnown.
	refs []byte
}

// catalogPacks reads the packs, several at once, and commits each from one
// goroutine, so two packs holding one key never race on its refs row.
func catalogPacks(ctx context.Context, opts *SnapshotOptions, todo []pendingPack, progress *SnapshotProgress) error {
	// Bound the reads in flight, and the parsed packs waiting to commit.
	reads := opts.Reads
	if reads <= 0 {
		reads = DefaultPackReads
	}
	eg, egCtx := errgroup.WithContext(ctx)
	ready := make(chan *readPack, readyPacks)

	// Read the packs, at most reads at once.
	eg.Go(func() error {
		// Close the queue once every read ended.
		defer close(ready)
		inner, innerCtx := errgroup.WithContext(egCtx)
		inner.SetLimit(reads)

		// Read each pack and queue it.
		for _, p := range todo {
			if innerCtx.Err() != nil {
				break
			}
			inner.Go(func() error {
				rp, err := readCatalogPack(innerCtx, opts.Packs, p)
				if err != nil {
					return err
				}
				select {
				case ready <- rp:
					return nil
				case <-innerCtx.Done():
					return context.Cause(innerCtx)
				}
			})
		}
		return inner.Wait()
	})

	// Commit the packs as they arrive, with those already waiting in the
	// same transaction, so a store of many small packs pays a commit per
	// batch rather than per pack.
	eg.Go(func() error {
		for rp := range ready {
			batch, keys := []*readPack{rp}, len(rp.keys)
		drain:
			for keys < commitKeys {
				select {
				case next, ok := <-ready:
					if !ok {
						break drain
					}
					batch = append(batch, next)
					keys += len(next.keys)
				default:
					break drain
				}
			}
			if err := commitCatalogPacks(egCtx, opts.Store, batch); err != nil {
				return err
			}
			for _, rp := range batch {
				progress.Catalogued++
				progress.Bytes += rp.row.size
			}
			if opts.Progress != nil {
				opts.Progress(*progress)
			}
		}
		return nil
	})
	return eg.Wait()
}

// readCatalogPack reads a pack whole and parses its keys and refs. A value
// whose bytes fail their key's hash is a corrupt copy, not an error: the
// catalog records it, and the walk calls a reached corrupt block a hole.
func readCatalogPack(ctx context.Context, src PackSource, p pendingPack) (*readPack, error) {
	// Read the pack.
	data, err := src.ReadPackFile(ctx, p.id, p.row.size)
	if err != nil {
		return nil, err
	}
	rd, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		return nil, errors.Wrap(err, "open pack "+p.id)
	}

	// Parse each key's copy.
	rp := &readPack{id: p.id, row: catalogPack{size: p.row.size, created: p.row.created}}
	err = rd.ScanPrefix(nil, func(key, value []byte) error {
		// Classify the copy by its hash and its refs.
		k := packKey{tail: bytes.Clone(Tail(key)), member: member{valueSize: uint64(len(value))}}
		_, stored, err := packfile.DecodeBlockValue(key, value)
		switch {
		case err != nil:
			k.state = refsCorrupt
			k.member.flags |= memberCorrupt
		case !stored.RefsKnown:
			k.state = refsUnknown
			k.member.flags |= memberRefsUnknown
		default:
			k.state = refsKnown
			if k.refs, err = marshalRefs(refsKnown, nonEmpty(stored.Refs)); err != nil {
				return err
			}
		}

		// Add it to the pack's keys and totals.
		rp.keys = append(rp.keys, k)
		rp.row.blocks++
		rp.row.valueBytes += uint64(len(value))
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan pack "+p.id)
	}
	return rp, nil
}

// nonEmpty drops the empty refs, which point at nothing.
func nonEmpty(refs []*block.BlockRef) []*block.BlockRef {
	out := refs[:0:0]
	for _, ref := range refs {
		if !ref.GetEmpty() {
			out = append(out, ref)
		}
	}
	return out
}

// commitCatalogPacks writes the packs' rows in one transaction, each pack's
// row with its keys, keeping each key's best refs row.
func commitCatalogPacks(ctx context.Context, store kvtx.Store, packs []*readPack) error {
	// Open a write.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Write each pack, then commit them together.
	for _, rp := range packs {
		if err := writeCatalogPack(ctx, tx, rp); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// writeCatalogPack writes a pack's member rows, its keys' refs rows where
// this copy is better, and the pack row.
func writeCatalogPack(ctx context.Context, tx kvtx.TxOps, rp *readPack) error {
	id := []byte(rp.id + "/")
	for _, k := range rp.keys {
		// Record the membership.
		if err := tx.Set(ctx, prefixed(catalogMemberPrefix, id, k.tail), k.member.marshal()); err != nil {
			return err
		}

		// Keep the better refs row.
		refsKey := prefixed(catalogRefsPrefix, k.tail)
		prev, found, err := tx.Get(ctx, refsKey)
		if err != nil {
			return err
		}
		if found && len(prev) != 0 && prev[0] >= k.state {
			continue
		}
		value := k.refs
		if value == nil {
			value = []byte{k.state}
		}
		if err := tx.Set(ctx, refsKey, value); err != nil {
			return err
		}
	}
	return tx.Set(ctx, []byte(catalogPackPrefix+rp.id), rp.row.marshal())
}
