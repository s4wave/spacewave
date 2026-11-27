package block_gc

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// holderParentLimit bounds the parents of each ref retained for the next scan.
const holderParentLimit = 8

// holderTarget retains one requested ref and its bounded parent sample.
type holderTarget struct {
	// ref is the child whose parents this level seeks.
	ref *block.BlockRef
	// parents holds the first parents in child-list key order.
	parents []*block.BlockRef
	// total counts all matching lists, including those beyond the sample.
	total uint64
}

// InspectHolders reads refs and scans stored child lists once per level in key order.
// up is the number of parent levels, from one through eight.
// visit receives the requested refs at level zero with a nil child, then every
// matching parent with the child its list references. Released lists are included;
// their Released flag shows that the list has already relinquished its holds.
// Only the first eight parents per ref are followed at the next level.
// After the walk, visit receives each named root holding a retained target at level -1.
// Root rows identify the held block in Ref and the name in Root, with a nil child.
// It keeps the requested refs and each level's bounded parent samples, never all lists.
// The callback must not open another transaction on this store.
// Cost: up streaming list scans and one root scan, plus bounded point reads per block row.
// All reads share one snapshot.
func (c *Collector) InspectHolders(
	ctx context.Context, refs []*block.BlockRef, up int,
	visit func(level int, child *block.BlockRef, row HolderRow) error,
) ([]HolderLevel, error) {
	// Reject an unsupported depth before opening the read snapshot.
	if up < 1 || up > 8 {
		return nil, errors.New("up must be between 1 and 8")
	}
	levels := make([]HolderLevel, 1, up+1)
	err := c.store.ViewBlocks(ctx, func(rec *Records, blocks block.StoreOps) error {
		// Deduplicate requested refs and print their structure in input order.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		targets := make(map[string]*holderTarget, len(refs))
		retained := make(map[string]*block.BlockRef, len(refs))
		for _, ref := range refs {
			key := string(BlockKey(ref))
			if targets[key] != nil {
				continue
			}
			targets[key] = &holderTarget{ref: ref}
			retained[key] = ref
			row, err := readHolderRow(ctx, rec, blocks, state.GetUpCursor(), ref)
			if err != nil {
				return err
			}
			if err := visit(0, nil, row); err != nil {
				return err
			}
			levels[0].Rows++
			levels[0].States[row.Cause]++
		}

		// Stream all matching lists, retaining only the first parents per target.
		for level := 1; level <= up && len(targets) != 0; level++ {
			counts := HolderLevel{}
			err := rec.scanKind(ctx, kindChildren, nil, func(key []byte, value func() ([]byte, error)) (bool, error) {
				// Inspect packed child keys directly instead of allocating a decoded list.
				packed, err := value()
				if err != nil {
					return false, err
				}
				packed = listKeys(packed)
				var parent *block.BlockRef
				var row HolderRow
				for len(packed) != 0 {
					child, ok := nextBlockKey(packed)
					if !ok {
						return false, errBadBlockKey
					}
					packed = packed[len(child):]
					target := targets[string(child)]
					if target == nil {
						continue
					}

					// Read the parent once and emit one relationship per matching child.
					if parent == nil {
						var rest []byte
						parent, rest, err = ParseBlockKey(key)
						if err != nil || len(rest) != 0 {
							return false, errBadBlockKey
						}
						row, err = readHolderRow(ctx, rec, blocks, state.GetUpCursor(), parent)
						if err != nil {
							return false, err
						}
					}
					if err := visit(level, target.ref, row); err != nil {
						return false, err
					}
					counts.States[row.Cause]++
					target.total++
					if len(target.parents) < holderParentLimit {
						target.parents = append(target.parents, parent)
					}
				}
				return true, nil
			})
			if err != nil {
				return err
			}

			// Follow the bounded parent samples, deduplicating shared ancestors.
			next := make(map[string]*holderTarget)
			for _, target := range targets {
				counts.Rows += target.total
				for _, parent := range target.parents {
					key := string(BlockKey(parent))
					next[key] = &holderTarget{ref: parent}
					retained[key] = parent
				}
			}
			counts.Followed = len(next)
			levels = append(levels, counts)
			targets = next
		}

		// Stream roots once against the retained targets from every level.
		return rec.ScanRoots(ctx, func(bucketID, name string, entry *RootEntry) error {
			ref := retained[string(entry.GetRef())]
			if ref == nil {
				return nil
			}
			return visit(-1, nil, HolderRow{Ref: ref, Root: &FrontRoot{BucketID: bucketID, Name: name}})
		})
	})
	return levels, err
}
