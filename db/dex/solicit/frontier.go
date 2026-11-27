package dex_solicit

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// maxFrontierRefs bounds the refs a session remembers from finished subtree
// streams, and so the roots of one frontier request.
const maxFrontierRefs = 1024

// frontierRef is a ref a subtree stream from the session's peer named and left
// unsent when it stopped at its budget.
type frontierRef struct {
	ref *block.BlockRef
	// fetch is the frontier request asking for ref, or nil before one does.
	fetch *frontierFetch
}

// frontierFetch is one frontier request: a subtree request for the blocks
// under several roots at once.
type frontierFetch struct {
	// done is closed once found is set.
	done chan struct{}
	// found maps the key of each root the stream sent to its block.
	found map[string]*block.PutBatchEntry
}

// rememberFrontier records refs, which a subtree stream from the peer named
// and left unsent at its budget, on the session's frontier, up to
// maxFrontierRefs. Each frontier request takes every ref no other one is
// asking for, so a ref never asked for leaves with the next one.
func (s *peerSession) rememberFrontier(refs []*block.BlockRef) {
	// Add each ref not already on the frontier while it has room.
	if len(refs) == 0 {
		return
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.frontier == nil {
		s.frontier = make(map[string]*frontierRef, len(refs))
	}
	for _, ref := range refs {
		if len(s.frontier) >= maxFrontierRefs {
			return
		}
		if key := ref.MapKey(); s.frontier[key] == nil {
			s.frontier[key] = &frontierRef{ref: ref}
		}
	}
}

// requestFrontier answers a subtree request for ref from the session's
// frontier. When no frontier request is asking for ref, it claims the
// fanout's subtree and asks the peer, in one request, for the blocks under
// ref and under every other frontier ref no request is asking for and the
// asker does not hold, ref first; a request for a ref such a request is
// asking for waits for its answer. It returns ref's block and, to the request
// that sent the frontier request, the other blocks the stream sent; ok is
// false when ref is not on the frontier, another peer claimed the subtree, or
// the stream did not send ref, so the caller asks for ref alone. The frontier
// request's roots leave the frontier, and what its stream left at its budget
// joins it.
func (s *peerSession) requestFrontier(ctx context.Context, ref *block.BlockRef, sub *subtreeAsk) (found *DexMessage, blocks []*block.PutBatchEntry, ok bool) {
	// Wait for the frontier request asking for ref, or claim the idle refs.
	key := ref.MapKey()
	f, roots := s.joinFrontier(key, sub)
	if f == nil {
		return nil, nil, false
	}
	if roots == nil {
		select {
		case <-ctx.Done():
			return nil, nil, false
		case <-f.done:
		}
		entry := f.found[key]
		return entryMessage(entry), nil, entry != nil
	}

	// Take the roots off the frontier and wake the waiters, whatever the
	// stream sent.
	rootKeys := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		rootKeys[root.MapKey()] = struct{}{}
	}
	var unsent []*block.BlockRef
	defer func() {
		// Drop the roots, wake their waiters, then remember the cut.
		s.mtx.Lock()
		for rootKey := range rootKeys {
			if e := s.frontier[rootKey]; e != nil && e.fetch == f {
				delete(s.frontier, rootKey)
			}
		}
		s.mtx.Unlock()
		close(f.done)
		s.rememberFrontier(unsent)
	}()

	// Ask for no root besides ref that the asker already holds.
	if sub.have != nil && len(roots) > 1 {
		if held, err := sub.have(ctx, roots[1:]); err == nil {
			kept := roots[:1]
			for i, ok := range held {
				if !ok {
					kept = append(kept, roots[1+i])
				}
			}
			roots = kept
		}
	}

	// Read the stream under the roots.
	req := &DexMessage{Refs: roots, SubtreeBytes: sub.budget}
	p, finish, err := s.openRequest(ctx, req, int(sub.budget))
	if err != nil {
		return nil, nil, false
	}
	msgs, err := s.take(ctx, p)
	if err != nil {
		finish(true)
		return nil, nil, false
	}
	streamed, cut, open := s.readSubtree(ctx, req.GetRequestId(), p, sub, roots, msgs)
	finish(open)
	unsent = cut

	// Hand each root its block, and the rest to this request.
	for _, entry := range streamed {
		entryKey := entry.Ref.MapKey()
		if _, isRoot := rootKeys[entryKey]; isRoot {
			f.found[entryKey] = entry
		}
		if entryKey != key {
			blocks = append(blocks, entry)
		}
	}
	entry := f.found[key]
	return entryMessage(entry), blocks, entry != nil
}

// joinFrontier looks up the frontier ref with key. It returns the frontier
// request already asking for it with nil roots, or, once sub claims the
// fanout's subtree, a new frontier request and the roots it takes: the ref
// first, then every frontier ref no request is asking for, up to
// maxFrontierRefs. It returns nil when the ref is not on the frontier or
// another peer claimed the subtree.
func (s *peerSession) joinFrontier(key string, sub *subtreeAsk) (*frontierFetch, []*block.BlockRef) {
	// Find the ref, and the request asking for it.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	e := s.frontier[key]
	if e == nil {
		return nil, nil
	}
	if e.fetch != nil {
		return e.fetch, nil
	}
	if sub.claim != nil && !sub.claim() {
		return nil, nil
	}

	// Take the ref and the idle refs into a new frontier request.
	f := &frontierFetch{done: make(chan struct{}), found: make(map[string]*block.PutBatchEntry)}
	e.fetch = f
	roots := []*block.BlockRef{e.ref}
	for _, other := range s.frontier {
		if len(roots) == maxFrontierRefs {
			break
		}
		if other.fetch == nil {
			other.fetch = f
			roots = append(roots, other.ref)
		}
	}
	return f, roots
}

// entryMessage returns the found response for a block a subtree stream sent,
// or nil for a nil entry.
func entryMessage(entry *block.PutBatchEntry) *DexMessage {
	if entry == nil {
		return nil
	}
	return &DexMessage{Ref: entry.Ref, Found: true, Data: entry.Data, Refs: entry.Refs, RefsKnown: true}
}
