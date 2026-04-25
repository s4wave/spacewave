package delta

import (
	"context"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	packfile_order "github.com/s4wave/spacewave/core/provider/spacewave/packfile/order"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// ExistsChecker is the subset of block.StoreOps that DiffBlockStores needs to
// filter the source iteration. *MirrorUnion and any block.Store implement it.
type ExistsChecker interface {
	// GetBlockExists reports whether a block is present.
	GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error)
}

// DiffBlockStores returns a =writer.BlockIterator= that yields every block
// present in =src= whose hash is NOT already present in =mirror=. Keys that
// fail to parse as a base58 =hash.Hash= are skipped silently (kvfile entries
// that are not hydra blocks).
//
// =mirror= may be nil; in that case every src block is emitted (mirror-absent
// degenerate case).
func DiffBlockStores(ctx context.Context, src *kvfile.Reader, mirror ExistsChecker) (writer.BlockIterator, error) {
	return DiffBlockStoresWithRefGraph(ctx, src, mirror, nil)
}

// DiffBlockStoresWithRefGraph returns a diff iterator ordered by GC graph
// locality when graph is available.
func DiffBlockStoresWithRefGraph(
	ctx context.Context,
	src *kvfile.Reader,
	mirror ExistsChecker,
	graph packfile_order.RefGraph,
) (writer.BlockIterator, error) {
	if src == nil {
		return nil, errors.New("src kvfile reader is nil")
	}

	// Pre-collect the index entries so the returned iterator can walk them
	// sequentially without holding scan-callback state across calls.
	type diffEntry struct {
		entry *kvfile.IndexEntry
		hash  *hash.Hash
	}

	var entries []diffEntry
	err := src.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		h := &hash.Hash{}
		if err := h.ParseFromB58(string(ie.GetKey())); err != nil {
			return nil
		}
		if mirror != nil {
			exists, err := mirror.GetBlockExists(ctx, &block.BlockRef{Hash: h})
			if err != nil {
				return errors.Wrap(err, "mirror exists probe")
			}
			if exists {
				return nil
			}
		}
		entries = append(entries, diffEntry{
			entry: ie.CloneVT(),
			hash:  h,
		})
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan src kvfile entries")
	}

	refs := make([]*block.BlockRef, 0, len(entries))
	byKey := make(map[string]diffEntry, len(entries))
	for _, entry := range entries {
		key := entry.hash.MarshalString()
		refs = append(refs, block.NewBlockRef(entry.hash))
		byKey[key] = entry
	}
	orderedRefs, err := packfile_order.BlockRefs(ctx, graph, refs)
	if err != nil {
		return nil, errors.Wrap(err, "order diff block refs")
	}
	entries = entries[:0]
	for _, ref := range orderedRefs {
		entry, ok := byKey[ref.GetHash().MarshalString()]
		if ok {
			entries = append(entries, entry)
		}
	}

	idx := 0
	return func() (*hash.Hash, []byte, error) {
		for idx < len(entries) {
			entry := entries[idx]
			idx++

			data, found, err := src.Get(entry.entry.GetKey())
			if err != nil {
				return nil, nil, errors.Wrap(err, "read src block")
			}
			if !found {
				continue
			}
			return entry.hash, data, nil
		}
		return nil, nil, nil
	}, nil
}
