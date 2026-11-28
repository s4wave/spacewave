package rootwalk

import (
	"context"
	"reflect"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world/refwalk"

	space_world "github.com/s4wave/spacewave/core/space/world"
)

// Walk walks every block sp retains: the root blocks its named roots hold,
// the base and head Worlds of its cursor with their retained roots, the World
// and payloads of each outcome, and the published graph. It walks every World
// before any payload, so a payload the Worlds reference as an object root is
// walked under its object's type, as refwalk.Walker.PayloadNode describes.
//
// w decodes the Worlds and payloads and keeps the blocks visited across
// walks, so walking several Spaces with one Walker visits a shared block
// once.
func Walk(ctx context.Context, w *refwalk.Walker, sp Space, v Visitor) error {
	s := &spaceWalk{w: w, sp: sp, v: v}
	return s.walk(ctx)
}

// spaceWalk is the state of one Walk.
type spaceWalk struct {
	// w decodes the blocks below the root blocks.
	w *refwalk.Walker
	// sp is the Space walked.
	sp Space
	// v receives the blocks.
	v Visitor
}

// walk collects the Space's Worlds and payload roots, walks each World, then
// each payload the Worlds did not type.
func (s *spaceWalk) walk(ctx context.Context) error {
	// Collect the Worlds and payloads the named roots hold.
	worlds, payloads, err := s.bucketRoots(ctx)
	if err != nil {
		return err
	}

	// Collect the cursor's Worlds with their retained roots, and the World and
	// payloads of each outcome.
	for _, state := range []*space_world.InnerState{s.sp.Cursor.GetBase(), s.sp.Cursor.GetHead()} {
		head := state.GetHeadRef()
		if head == nil {
			continue
		}
		refs := []*block.BlockRef{head.GetRootRef()}
		for _, retained := range state.GetRetainedRoots() {
			refs = append(refs, retained.GetRootRef())
		}
		for _, ref := range refs {
			node, err := s.w.WorldNode(head, ref)
			if err != nil {
				return err
			}
			worlds = append(worlds, node)
		}
	}
	for _, outcome := range s.sp.Cursor.GetOutcomes() {
		node, err := s.w.WorldNode(s.sp.World, outcome.GetWorld())
		if err != nil {
			return err
		}
		worlds = append(worlds, node)
		payloads = append(payloads, outcome.GetPayloads()...)
	}

	// Walk each World.
	for _, root := range worlds {
		if err := s.w.Walk(ctx, root, s.v.Blocks); err != nil {
			return err
		}
	}

	// Walk each payload, with the roots of the published graph that the
	// Worlds did not reach.
	published, err := s.publishedRoots(ctx)
	if err != nil {
		return err
	}
	payloads = append(payloads, published...)
	for _, ref := range payloads {
		root, ok, err := s.w.PayloadNode(ctx, s.sp.World, ref)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := s.w.Walk(ctx, root, s.v.Blocks); err != nil {
			return err
		}
	}
	return nil
}

// bucketRoots reads every root the Space holds by name, visiting each root
// block, and returns the Worlds and payload roots those blocks reference.
func (s *spaceWalk) bucketRoots(ctx context.Context) ([]refwalk.Node, []*block.BlockRef, error) {
	// Each named root adds its Worlds or payload roots.
	var worlds []refwalk.Node
	var payloads []*block.BlockRef
	addWorlds := func(refs []*block.BlockRef) error {
		for _, ref := range refs {
			node, err := s.w.WorldNode(s.sp.World, ref)
			if err != nil {
				return err
			}
			worlds = append(worlds, node)
		}
		return nil
	}

	// Decode each root by the type its name selects.
	for _, name := range RootNames {
		// Read the current holder.
		ref, err := s.sp.GetRoot(ctx, name)
		if err != nil {
			return nil, nil, err
		}
		if ref.GetEmpty() {
			continue
		}

		// Release the legacy span instead of walking it when asked, so the
		// walk never visits the discarded chain.
		if name == LegacyReplaySpanRoot && s.sp.ReleaseLegacySpan != nil {
			if err := s.sp.ReleaseLegacySpan(ctx, ref); err != nil {
				return nil, nil, err
			}
			continue
		}

		// Collect the root's Worlds or payloads.
		var roots, spanPayloads []*block.BlockRef
		switch name {
		case space_world.RootReplayBase:
			roots = []*block.BlockRef{ref}
		case space_world.RootRetained:
			roots, err = s.retainedRoots(ctx, ref)
		case space_world.RootReplayPayloads:
			spanPayloads, err = s.heldRoots(ctx, ref)
		case LegacyReplaySpanRoot:
			roots, spanPayloads, err = s.spanRoots(ctx, ref)
		default:
			roots, err = s.heldRoots(ctx, ref)
		}
		if err == nil {
			err = addWorlds(roots)
		}
		if err != nil {
			return nil, nil, err
		}
		payloads = append(payloads, spanPayloads...)
	}
	return worlds, payloads, nil
}

// publishedRoots reads the root holding the published graph, which is the
// root space_world.PublishedGraph gives: the World of the published
// checkpoint, one operation's payload root, or a block referencing that World
// and every payload root. It visits that block and returns the roots the walk
// has not reached under a type, to walk as payloads.
//
// Call it after walking the Space's Worlds, since nothing here says which
// root is the World: a World that no other root holds is then read as a
// payload, which is not certainly a blob, so the Walker reads no refs for it.
func (s *spaceWalk) publishedRoots(ctx context.Context) ([]*block.BlockRef, error) {
	// Skip a root the walk reached under a type, a World it has walked.
	if s.sp.PublishedRoot == "" {
		return nil, nil
	}
	ref, err := s.sp.GetRoot(ctx, s.sp.PublishedRoot)
	if err != nil || ref.GetEmpty() {
		return nil, err
	}
	if typed, _, err := s.w.Reached(ctx, ref); err != nil || typed {
		return nil, err
	}

	// Take any other root that is not exactly the block referencing several
	// roots as the graph's one root.
	data, found, err := s.readRoot(ctx, ref, &space_world.HeldRoots{})
	if err != nil || !found {
		return nil, err
	}
	roots, ok := space_world.PublishedRoots(data)
	if !ok {
		return []*block.BlockRef{ref}, nil
	}
	if err := s.visitRoot(ctx, ref, &space_world.HeldRoots{}, roots, nil); err != nil {
		return nil, err
	}

	// Keep the roots the Worlds did not reach, in a slice of their own: the
	// visitor may hold roots.
	var rest []*block.BlockRef
	for _, root := range roots {
		typed, _, err := s.w.Reached(ctx, root)
		if err != nil {
			return nil, err
		}
		if !typed {
			rest = append(rest, root)
		}
	}
	return rest, nil
}

// heldRoots reads the HeldRoots chain at ref, visiting each block, and returns
// its roots.
func (s *spaceWalk) heldRoots(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, error) {
	var roots []*block.BlockRef
	for !ref.GetEmpty() {
		// Read and decode the next block of the chain.
		msg := &space_world.HeldRoots{}
		data, found, err := s.readRoot(ctx, ref, msg)
		if err != nil || !found {
			return roots, err
		}
		if err := refwalk.DecodeExact(msg, data); err != nil {
			return roots, s.visitRoot(ctx, ref, msg, nil, err)
		}

		// Visit it and follow its previous block.
		if err := s.visitRoot(ctx, ref, msg, appendRef(msg.GetRoots(), msg.GetPrev()), nil); err != nil {
			return nil, err
		}
		roots = append(roots, msg.GetRoots()...)
		ref = msg.GetPrev()
	}
	return roots, nil
}

// retainedRoots reads the RetainedRootSet at ref, visiting it, and returns its
// World roots.
func (s *spaceWalk) retainedRoots(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, error) {
	// Read and decode the set.
	msg := &space_world.RetainedRootSet{}
	data, found, err := s.readRoot(ctx, ref, msg)
	if err != nil || !found {
		return nil, err
	}
	if err := refwalk.DecodeExact(msg, data); err != nil {
		return nil, s.visitRoot(ctx, ref, msg, nil, err)
	}

	// Visit it with its World roots.
	roots := make([]*block.BlockRef, 0, len(msg.GetRoots()))
	for _, root := range msg.GetRoots() {
		roots = appendRef(roots, root.GetRootRef())
	}
	return roots, s.visitRoot(ctx, ref, msg, roots, nil)
}

// spanRoots reads the ReplaySpan chain at ref, visiting each block, and
// returns its Worlds and payload roots.
func (s *spaceWalk) spanRoots(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, []*block.BlockRef, error) {
	var worlds, payloads []*block.BlockRef
	for !ref.GetEmpty() {
		// Read and decode the next span.
		span := &space_world.ReplaySpan{}
		data, found, err := s.readRoot(ctx, ref, span)
		if err != nil || !found {
			return worlds, payloads, err
		}
		if err := refwalk.DecodeExact(span, data); err != nil {
			return worlds, payloads, s.visitRoot(ctx, ref, span, nil, err)
		}

		// Visit it and follow its previous span.
		refs := appendRef(slices.Concat(span.GetWorlds(), span.GetPayloads()), span.GetPrev())
		if err := s.visitRoot(ctx, ref, span, refs, nil); err != nil {
			return nil, nil, err
		}
		worlds = append(worlds, span.GetWorlds()...)
		payloads = append(payloads, span.GetPayloads()...)
		ref = span.GetPrev()
	}
	return worlds, payloads, nil
}

// readRoot reads the root block at ref, which decodes as msg, and visits it
// as absent when RootBlocks lacks it.
func (s *spaceWalk) readRoot(ctx context.Context, ref *block.BlockRef, msg any) ([]byte, bool, error) {
	data, found, err := s.sp.RootBlocks.GetBlock(ctx, ref)
	if err != nil || found {
		return data, found, err
	}
	return nil, false, s.v.Root(ctx, RootBlock{Ref: ref, Type: typeName(msg)})
}

// visitRoot visits the found root block at ref, decoded as msg with refs or
// undecodable with decodeErr.
func (s *spaceWalk) visitRoot(ctx context.Context, ref *block.BlockRef, msg any, refs []*block.BlockRef, decodeErr error) error {
	return s.v.Root(ctx, RootBlock{Ref: ref, Type: typeName(msg), Found: true, DecodeErr: decodeErr, Refs: refs})
}

// appendRef appends ref to refs unless it is empty.
func appendRef(refs []*block.BlockRef, ref *block.BlockRef) []*block.BlockRef {
	if ref.GetEmpty() {
		return refs
	}
	return append(refs, ref)
}

// typeName returns the Go type of a decoded root block.
func typeName(msg any) string {
	return reflect.TypeOf(msg).String()
}
