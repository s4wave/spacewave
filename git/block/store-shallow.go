package git_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// SetShallow sets the list of shallow refs.
func (r *Store) SetShallow(refs []plumbing.Hash) error {
	_, bcs, err := r.root.FollowShallowRefsStore(r.ctx, r.bcs)
	if err != nil {
		return err
	}
	if bcs == nil {
		return block.ErrUnexpectedType
	}
	nb, err := NewShallowRefsStore(refs)
	if err != nil {
		return err
	}
	bcs.SetBlock(nb, true)
	return nil
}

// Shallow returns the list of shallow refs.
func (r *Store) Shallow() ([]plumbing.Hash, error) {
	shallowStore, _, err := r.root.FollowShallowRefsStore(r.ctx, r.bcs)
	if err != nil {
		return nil, err
	}
	return FromHashSet(shallowStore.GetShallowRefs())
}

// _ is a type assertion
var _ storer.ShallowStorer = (*Store)(nil)
