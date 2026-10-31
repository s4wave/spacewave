package block

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
)

// DeadlistRootPrefix prefixes the named root of each deadlist record.
const DeadlistRootPrefix = "deadlist/"

// DeadlistDueRootPrefix prefixes the named root of each due deadlist record,
// which the collector handles and releases.
const DeadlistDueRootPrefix = "deadlist-due/"

// maxDeadlistPartRefs bounds the refs one Deadlist block lists, so a block
// stays near 3 MB, well below MaxBlockSize.
const maxDeadlistPartRefs = 1 << 16

// DeadlistRootName returns the named root of the pending record at seq. The
// sequence is zero padded so the names sort in sequence order.
func DeadlistRootName(seq uint64) string {
	return fmt.Sprintf("%s%020d", DeadlistRootPrefix, seq)
}

// DeadlistDueRootName returns the named root of the due record at seq.
func DeadlistDueRootName(seq uint64) string {
	return fmt.Sprintf("%s%020d", DeadlistDueRootPrefix, seq)
}

// EncodeDeadlist encodes the record at seq of what died from the named World
// at from to the one at to, as the first block and its parts. It returns the
// entries to store and the first block's ref, which the record's name holds.
// The entries declare no refs to the pages they list.
func EncodeDeadlist(seq uint64, from, to *BlockRef, dead *Replacements) ([]*PutBatchEntry, *BlockRef, error) {
	// Split the refs into parts of at most maxDeadlistPartRefs each.
	parts := []*Deadlist{{Seq: seq, From: from, To: to}}
	n := 0
	add := func(ref *BlockRef, list func(*Deadlist) *[]*BlockRef) {
		if n == maxDeadlistPartRefs {
			parts, n = append(parts, &Deadlist{Seq: seq}), 0
		}
		p := list(parts[len(parts)-1])
		*p = append(*p, ref)
		n++
	}
	for _, ref := range dead.Replaced {
		add(ref, func(d *Deadlist) *[]*BlockRef { return &d.Replaced })
	}
	for _, ref := range dead.Removed {
		add(ref, func(d *Deadlist) *[]*BlockRef { return &d.Removed })
	}
	for _, ref := range dead.Kept {
		add(ref, func(d *Deadlist) *[]*BlockRef { return &d.Kept })
	}
	for _, ref := range dead.Held {
		add(ref, func(d *Deadlist) *[]*BlockRef { return &d.Held })
	}
	for _, ref := range dead.Existed {
		add(ref, func(d *Deadlist) *[]*BlockRef { return &d.Existed })
	}

	// Encode the parts after the first, then the first naming them.
	var entries []*PutBatchEntry
	for _, p := range parts[1:] {
		entry, err := encodeDeadlistBlock(p, nil)
		if err != nil {
			return nil, nil, err
		}
		parts[0].Parts = append(parts[0].Parts, entry.Ref)
		entries = append(entries, entry)
	}
	first, err := encodeDeadlistBlock(parts[0], parts[0].Parts)
	if err != nil {
		return nil, nil, err
	}
	return append(entries, first), first.Ref, nil
}

// encodeDeadlistBlock encodes msg as a block that declares refs.
func encodeDeadlistBlock(msg interface{ MarshalVT() ([]byte, error) }, refs []*BlockRef) (*PutBatchEntry, error) {
	// Encode it and address the data.
	data, err := msg.MarshalVT()
	if err != nil {
		return nil, err
	}
	ref, err := BuildBlockRef(data, nil)
	if err != nil {
		return nil, err
	}
	return &PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}, nil
}

// ReadDeadlist reads the record whose first block is ref from src and returns
// its first block, with the refs of every part gathered into it, as
// Replacements.
func ReadDeadlist(ctx context.Context, src StoreOps, ref *BlockRef) (*Deadlist, *Replacements, error) {
	// Read the first block, then each part.
	first, err := readDeadlistBlock(ctx, src, ref)
	if err != nil {
		return nil, nil, err
	}
	dead := &Replacements{Replaced: first.Replaced, Removed: first.Removed, Kept: first.Kept, Held: first.Held, Existed: first.Existed}
	for _, partRef := range first.Parts {
		part, err := readDeadlistBlock(ctx, src, partRef)
		if err != nil {
			return nil, nil, err
		}
		dead.Replaced = append(dead.Replaced, part.Replaced...)
		dead.Removed = append(dead.Removed, part.Removed...)
		dead.Kept = append(dead.Kept, part.Kept...)
		dead.Held = append(dead.Held, part.Held...)
		dead.Existed = append(dead.Existed, part.Existed...)
	}
	return first, dead, nil
}

// readDeadlistBlock reads and decodes one Deadlist block.
func readDeadlistBlock(ctx context.Context, src StoreOps, ref *BlockRef) (*Deadlist, error) {
	// Read the block and decode it.
	data, found, err := src.GetBlock(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.Wrap(ErrNotFound, "deadlist block")
	}
	d := &Deadlist{}
	if err := d.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "deadlist block")
	}
	return d, nil
}
