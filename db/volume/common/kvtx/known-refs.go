package kvtx

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// requireRefsKnown refuses a write holding a block whose writer did not declare
// its child list, so every block the Volume stores carries one and liveness
// never meets a block that hides its children. A put of a block the Volume
// already stores is refused too: the check reads only the entries, so the put
// path reads nothing from the store to decide it. A tombstone and an empty
// entry store no block and pass. Returns block.ErrRefsUnknown naming the first
// refused block.
func requireRefsKnown(entries []*block.PutBatchEntry) error {
	for _, entry := range entries {
		if entry.Tombstone || entry.RefsKnown || len(entry.Data) == 0 {
			continue
		}
		return refsUnknown(entry.Ref, len(entry.Data))
	}
	return nil
}

// requireRefKnown is requireRefsKnown for one block written by PutBlock.
func requireRefKnown(data []byte, opts *block.PutOpts) error {
	if opts.GetRefsKnown() || len(data) == 0 {
		return nil
	}
	return refsUnknown(opts.GetForceBlockRef(), len(data))
}

// refsUnknown returns block.ErrRefsUnknown naming the refused block, by its ref
// when the writer gave one and by its size otherwise.
func refsUnknown(ref *block.BlockRef, size int) error {
	if !ref.GetEmpty() {
		return errors.Wrap(block.ErrRefsUnknown, ref.MarshalString())
	}
	return errors.Wrapf(block.ErrRefsUnknown, "block of %d bytes", size)
}
