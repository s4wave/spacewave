package block_mock

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// CheckReplacements compares a transaction's Replacements for a write from the
// root from to the root to against the diff of the two trees in src. It
// returns an error naming a page the diff finds dead that the record misses,
// or a page the record lists that the new tree still reaches. The record may
// also list pages the interval created and dropped, which neither tree reaches.
func CheckReplacements(ctx context.Context, src block.StoreOps, from, to *block.BlockRef, rep *block.Replacements) error {
	// A recording transaction always returns a record.
	if rep == nil {
		return errors.New("transaction recorded no replacements")
	}

	// Collect what the record reports dead.
	dead := make(map[string]struct{})
	for _, ref := range rep.Replaced {
		dead[ref.MarshalString()] = struct{}{}
	}
	err := rep.Expand(ctx, src, func(ref *block.BlockRef) error {
		dead[ref.MarshalString()] = struct{}{}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "expand removed subtrees")
	}

	// Every page the diff finds dead must be reported.
	err = block.DiffGraph(ctx, src, []*block.BlockRef{from}, []*block.BlockRef{to}, func(ref *block.BlockRef) error {
		if _, ok := dead[ref.MarshalString()]; !ok {
			return errors.Errorf("dead page %s not reported", ref.MarshalString())
		}
		return nil
	})
	if err != nil {
		return err
	}

	// No reported page may be live.
	return block.WalkGraph(ctx, src, []*block.BlockRef{to}, 0, func(ref *block.BlockRef, _ *block.StoredBlock) error {
		if _, ok := dead[ref.MarshalString()]; ok {
			return errors.Errorf("live page %s reported dead", ref.MarshalString())
		}
		return nil
	})
}

// CheckExisted compares the record's Existed list for a write from the root
// from to the root to against the trees in src. puts holds, by MarshalString,
// each page the interval put, true when the put found it stored, as Net takes
// it. A page is written when a put stored it afresh, or found it within the
// old tree, since that put was counted; any other page the new tree reaches
// already existed. The applier reads Existed for the
// live pages it gives a count change: those in Kept, the unlisted children of
// the Replaced pages, and the old root. Each such page the new tree reaches,
// other than its root, must be in Existed exactly when no written page names
// it, since only then was its edge from the new tree counted already.
func CheckExisted(ctx context.Context, src block.StoreOps, from, to *block.BlockRef, rep *block.Replacements, puts map[string]bool) error {
	// Index the existed list and the listed pages.
	existed := make(map[string]struct{}, len(rep.Existed))
	for _, ref := range rep.Existed {
		existed[ref.MarshalString()] = struct{}{}
	}
	listed := make(map[string]struct{}, len(rep.Replaced)+len(rep.Removed))
	for _, ref := range slices.Concat(rep.Replaced, rep.Removed) {
		listed[ref.MarshalString()] = struct{}{}
	}

	// Gather the written pages.
	written := make(map[string]struct{}, len(puts))
	for key, found := range puts {
		if !found {
			written[key] = struct{}{}
		}
	}
	var oldRoots []*block.BlockRef
	if !from.GetEmpty() {
		oldRoots = append(oldRoots, from)
	}
	err := block.WalkGraph(ctx, src, oldRoots, 0, func(ref *block.BlockRef, _ *block.StoredBlock) error {
		if puts[ref.MarshalString()] {
			written[ref.MarshalString()] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "walk the old tree")
	}

	// Gather the pages the applier gives a count change.
	checked := make(map[string]struct{})
	for _, ref := range rep.Kept {
		checked[ref.MarshalString()] = struct{}{}
	}
	if !from.GetEmpty() {
		checked[from.MarshalString()] = struct{}{}
	}
	err = block.WalkGraph(ctx, src, rep.Replaced, 0, func(ref *block.BlockRef, stored *block.StoredBlock) error {
		for _, child := range stored.Refs {
			if _, isListed := listed[child.MarshalString()]; !isListed {
				checked[child.MarshalString()] = struct{}{}
			}
		}
		return block.ErrSkipRefs
	})
	if err != nil {
		return errors.Wrap(err, "read replaced child lists")
	}

	// Mark each page the new tree reaches, and those a written page names.
	live := make(map[string]struct{})
	direct := make(map[string]struct{})
	err = block.WalkGraph(ctx, src, []*block.BlockRef{to}, 0, func(ref *block.BlockRef, stored *block.StoredBlock) error {
		key := ref.MarshalString()
		live[key] = struct{}{}
		if _, isWritten := written[key]; isWritten {
			for _, child := range stored.Refs {
				direct[child.MarshalString()] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "walk the new tree")
	}

	// Compare each live checked page's membership with its parents.
	for key := range checked {
		_, isLive := live[key]
		_, isExisted := existed[key]
		_, isDirect := direct[key]
		switch {
		case !isLive || key == to.MarshalString():
		case isDirect && isExisted:
			return errors.Errorf("page %s named by a written page listed as existed", key)
		case !isDirect && !isExisted:
			return errors.Errorf("page %s named only by existing pages not listed as existed", key)
		}
	}
	return nil
}
