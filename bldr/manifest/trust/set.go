// Package bldr_manifest_trust holds the manifest roots a host trusts to run in
// the page: each is a plugin ID paired with the manifest root it was trusted
// at, so a trusted root never lends its trust to another plugin ID.
package bldr_manifest_trust

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
)

// Root is a plugin ID paired with a manifest root, the b58 hash of the
// manifest's root block as it appears in a pinned asset path.
type Root struct {
	// PluginID is the plugin the manifest declares.
	PluginID string
	// ManifestRoot is the manifest root block hash.
	ManifestRoot string
}

// Set is an add-only set of trusted roots.
//
// A root never leaves the set. It identifies immutable content, and it entered
// for a fact that stays true of that content: the binary embeds it, or a
// release key the host trusted at the time signed it. A release pin change
// takes effect for roots verified after the change. A root verified earlier
// stays trusted until the process exits, so that trust is bounded by the page's
// lifetime.
type Set struct {
	// bcast guards the fields below and wakes watchers on an add.
	bcast broadcast.Broadcast
	// roots holds the roots in insertion order.
	roots []Root
	// index holds the roots for membership checks.
	index map[Root]struct{}
}

// NewSet constructs an empty Set.
func NewSet() *Set {
	return &Set{index: make(map[Root]struct{})}
}

// Add inserts r and reports whether it was new. The caller states beside the
// call the fact that makes r trusted.
func (s *Set) Add(r Root) bool {
	// Insert r once and wake the watchers.
	l := s.bcast.Lock()
	defer l.Unlock()
	if _, ok := s.index[r]; ok {
		return false
	}
	s.index[r] = struct{}{}
	s.roots = append(s.roots, r)
	l.Broadcast()
	return true
}

// Has reports whether r is in the set.
func (s *Set) Has(r Root) bool {
	// Check membership under the lock.
	l := s.bcast.Lock()
	defer l.Unlock()
	_, ok := s.index[r]
	return ok
}

// Watch calls cb with every root in the set, then with each batch of roots
// added after, until ctx ends or cb returns an error. The first call may be
// empty. Later calls never are.
func (s *Set) Watch(ctx context.Context, cb func(added []Root) error) error {
	var seen int
	first := true
	for {
		// Take the roots added since the last call, or wait for one.
		l := s.bcast.Lock()
		added := slices.Clone(s.roots[seen:])
		var waitCh <-chan struct{}
		if len(added) == 0 && !first {
			waitCh = l.WaitCh()
		}
		l.Unlock()

		// Wait for the next add when nothing is new.
		if waitCh != nil {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case <-waitCh:
			}
			continue
		}

		// Report the new roots.
		seen += len(added)
		first = false
		if err := cb(added); err != nil {
			return err
		}
	}
}
