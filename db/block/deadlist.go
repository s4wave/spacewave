package block

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"sort"

	"github.com/pkg/errors"
)

// DeadlistRootPrefix prefixes the named root of each deadlist record.
const DeadlistRootPrefix = "deadlist/"

// DeadlistDueRootPrefix prefixes the named root of each due deadlist record,
// which the collector handles and releases.
const DeadlistDueRootPrefix = "deadlist-due/"

// MaxDeadlistPartRefs bounds both a leaf's list references and an index's
// fanout. Each immutable preparation write has bounded graph work; larger
// records compose these same blocks into an index tree.
const MaxDeadlistPartRefs = 1024

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
// List leaves declare only their distinct Found pages as ordinary children.
// Headers and indexes declare their Parts; other historical lists are values.
// Each list is sorted on a copy so map iteration order cannot change its hash.
// It yields between bounded CPU slices; callers must run it off their writer.
func EncodeDeadlist(seq uint64, from, to *BlockRef, dead *Replacements) ([]*PutBatchEntry, *BlockRef, error) {
	// List data lives only in bounded leaves. Metadata stays in the header,
	// so naming and recount never decode a whole record under the writer.
	parts := []*Deadlist{{}}
	n := 0
	add := func(ref *BlockRef, list func(*Deadlist) *[]*BlockRef) {
		if n == MaxDeadlistPartRefs {
			runtime.Gosched()
			parts, n = append(parts, &Deadlist{}), 0
		}
		p := list(parts[len(parts)-1])
		*p = append(*p, ref)
		n++
	}
	for _, field := range []struct {
		refs []*BlockRef
		list func(*Deadlist) *[]*BlockRef
	}{
		{dead.Replaced, func(d *Deadlist) *[]*BlockRef { return &d.Replaced }},
		{dead.Removed, func(d *Deadlist) *[]*BlockRef { return &d.Removed }},
		{dead.Kept, func(d *Deadlist) *[]*BlockRef { return &d.Kept }},
		{dead.Held, func(d *Deadlist) *[]*BlockRef { return &d.Held }},
		{dead.Existed, func(d *Deadlist) *[]*BlockRef { return &d.Existed }},
		{dead.Dropped, func(d *Deadlist) *[]*BlockRef { return &d.Dropped }},
		{dead.Found, func(d *Deadlist) *[]*BlockRef { return &d.Found }},
		{dead.FallbackHeld, func(d *Deadlist) *[]*BlockRef { return &d.FallbackHeld }},
	} {
		// Canonicalize this value list without changing the sealed net's input.
		refs := slices.Clone(field.refs)
		comparisons := 0
		sort.Slice(refs, func(i, j int) bool {
			// Let foreground tasks run even when the target has one event loop.
			comparisons++
			if comparisons == MaxDeadlistPartRefs {
				comparisons = 0
				runtime.Gosched()
			}

			// Compare immutable hashes without allocating an encoded key.
			return refs[i].LessThan(refs[j])
		})
		for _, ref := range refs {
			add(ref, field.list)
		}
	}

	// Repeated logical chunks may share one physical block. Keep their index
	// occurrences for list multiplicity, but prepare each physical block once.
	var entries []*PutBatchEntry
	seen := make(map[string]struct{})
	encode := func(part *Deadlist, refs []*BlockRef, holds bool) (*BlockRef, error) {
		// Encode this physical payload once, preserving logical references above.
		entry, err := encodeDeadlistBlock(part, distinctDeadlistRefs(refs))
		if err != nil {
			return nil, err
		}
		entry.HoldsRoots = holds
		key := entry.Ref.MarshalString()
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			entries = append(entries, entry)
		}
		runtime.Gosched()
		return entry.Ref, nil
	}
	var level []*BlockRef
	if n != 0 {
		for _, part := range parts {
			ref, err := encode(part, part.Found, len(part.Found) != 0)
			if err != nil {
				return nil, nil, err
			}
			level = append(level, ref)
		}
	}

	// Index nodes use ordinary Parts references, never synthetic Found claims.
	// Leaves and indexes precede their parents in preparation order.
	for len(level) > MaxDeadlistPartRefs {
		var next []*BlockRef
		for len(level) != 0 {
			chunk := level[:min(len(level), MaxDeadlistPartRefs)]
			ref, err := encode(&Deadlist{Parts: chunk}, chunk, false)
			if err != nil {
				return nil, nil, err
			}
			next, level = append(next, ref), level[len(chunk):]
		}
		level = next
	}
	header := &Deadlist{Seq: seq, From: from, To: to, CountingGeneration: dead.CountingGeneration, CountsComplete: dead.CountsComplete, Parts: level}
	ref, err := encode(header, level, false)
	if err != nil {
		return nil, nil, err
	}
	return entries, ref, nil
}

// distinctDeadlistRefs bounds its set to one leaf or index, not the record.
func distinctDeadlistRefs(refs []*BlockRef) []*BlockRef {
	out := make([]*BlockRef, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		key := ref.MarshalString()
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			out = append(out, ref)
		}
	}
	return out
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
	// Aggregation is for off-writer interval resolution. Sharing and naming
	// read only the header, and physical preparation consumes bounded blocks.
	var first *Deadlist
	dead := &Replacements{}
	err := WalkDeadlist(ctx, src, ref, func(part *Deadlist) error {
		if first == nil {
			first = part
			dead.CountsComplete, dead.CountingGeneration = part.CountsComplete, part.CountingGeneration
		}
		appendDeadlistFields(dead, part)
		return nil
	})
	return first, dead, err
}

// ReadSmallDeadlist reads at most the header and one bounded list leaf. A
// false small result needs no list aggregation or traversal under the writer.
func ReadSmallDeadlist(ctx context.Context, src StoreOps, ref *BlockRef) (*Deadlist, *Replacements, bool, error) {
	// Read the immutable header and reject larger index shapes cheaply.
	first, err := ReadDeadlistBlock(ctx, src, ref)
	if err != nil {
		return nil, nil, false, err
	}
	if len(first.Parts) > 1 {
		return first, nil, false, nil
	}
	parts := []*Deadlist{first}
	if len(first.Parts) != 0 {
		leaf, err := ReadDeadlistBlock(ctx, src, first.Parts[0])
		if err != nil {
			return first, nil, false, err
		}
		if len(leaf.Parts) != 0 {
			return first, nil, false, nil
		}
		parts = append(parts, leaf)
	}

	// Preserve the same logical field order as the streaming reader.
	dead := &Replacements{CountsComplete: first.CountsComplete, CountingGeneration: first.CountingGeneration}
	for _, part := range parts {
		appendDeadlistFields(dead, part)
	}
	return first, dead, dead.RefCount() <= MaxDeadlistPartRefs, nil
}

// appendDeadlistFields appends one bounded leaf's list occurrences.
func appendDeadlistFields(dead *Replacements, part *Deadlist) {
	// Append values without changing either physical ownership or list order.
	dead.Replaced = append(dead.Replaced, part.Replaced...)
	dead.Removed = append(dead.Removed, part.Removed...)
	dead.Kept = append(dead.Kept, part.Kept...)
	dead.Held = append(dead.Held, part.Held...)
	dead.Existed = append(dead.Existed, part.Existed...)
	dead.Dropped = append(dead.Dropped, part.Dropped...)
	dead.Found = append(dead.Found, part.Found...)
	dead.FallbackHeld = append(dead.FallbackHeld, part.FallbackHeld...)
}

// WalkDeadlist streams a bounded index tree in logical list order. Repeated
// parts are visited for every occurrence, preserving Found multiplicity while
// their ordinary physical child edges remain a set.
func WalkDeadlist(ctx context.Context, src StoreOps, ref *BlockRef, visit func(*Deadlist) error) error {
	// Keep only the bounded pending fanout along the current index path.
	stack := []*BlockRef{ref}
	for len(stack) != 0 {
		partRef := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		part, err := ReadDeadlistBlock(ctx, src, partRef)
		if err != nil {
			return err
		}
		if err := visit(part); err != nil {
			return err
		}
		for _, child := range slices.Backward(part.Parts) {
			stack = append(stack, child)
		}
	}
	return nil
}

// ReadDeadlistBlock reads one bounded header, index or list leaf.
func ReadDeadlistBlock(ctx context.Context, src StoreOps, ref *BlockRef) (*Deadlist, error) {
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
