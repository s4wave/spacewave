package world_types

import (
	"context"
	"io"
	"slices"

	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// ObjectTypePageLimit bounds typed pages, including the default traversal page.
const ObjectTypePageLimit uint32 = 1000

// ListObjectsWithTypeOnGraph returns distinct subjects in the store's order:
// creation order on a world.EdgePager, index order on a Cayley kv store.
// startAfter is the previous page's last key. An empty key begins the listing.
// limit must be non-zero and is capped at ObjectTypePageLimit. The World must
// remain unchanged to retain exactly-once traversal across pages.
func ListObjectsWithTypeOnGraph(ctx context.Context, h graph.QuadStore, typeID, startAfter string, limit uint32) ([]string, bool, error) {
	// Validate the page before reading the graph.
	if typeID == "" {
		return nil, false, ErrTypeIDEmpty
	}
	if limit == 0 {
		return nil, false, errors.New("limit must be non-zero")
	}
	limit = min(limit, ObjectTypePageLimit)

	// Resolve the type and predicate to the store's node refs.
	typeRef, err := h.ValueOf(ctx, BuildTypeQuadValue(typeID))
	if err != nil || typeRef == nil {
		return nil, false, err
	}
	predRef, err := h.ValueOf(ctx, TypePred)
	if err != nil || predRef == nil {
		return nil, false, err
	}

	// Resolve the cursor to its subject node.
	var after graph.Ref
	if startAfter != "" {
		after, err = h.ValueOf(ctx, world.KeyToGraphValue(startAfter))
		if err != nil {
			return nil, false, err
		}
		if after == nil {
			return nil, false, errors.New("start_after has no graph node")
		}
	}

	// Read at most one page plus its lookahead.
	keys := make([]string, 0, limit)
	var more bool
	err = iterateSubjectsAfter(ctx, h, typeRef, predRef, after, func(ref graph.Ref) (bool, error) {
		// Detect a following page without decoding its first subject.
		if len(keys) == int(limit) {
			more = true
			return false, nil
		}

		// Decode the subject from its node reference.
		value, err := h.NameOf(ctx, ref)
		if err != nil {
			return false, err
		}
		key, ok := value.(quad.IRI)
		if !ok {
			return false, errors.New("type subject is not an object key")
		}
		keys = append(keys, string(key))
		return true, nil
	})
	return keys, more, err
}

// iterateSubjectsAfter calls cb with each distinct subject of the quads with
// obj and pred, in the store's order, after the subject after, or from the
// first when after is nil, until cb returns false or an error.
func iterateSubjectsAfter(ctx context.Context, h graph.QuadStore, obj, pred, after graph.Ref, cb func(subject graph.Ref) (bool, error)) error {
	switch s := h.(type) {
	case world.EdgePager:
		// Report each subject once, though labels repeat its edge.
		var last graph.Ref
		return s.IterateEdges(ctx, obj, pred, quad.Object, after, false, func(subject, _ graph.Ref) (bool, error) {
			if last != nil && subject.Key() == last.Key() {
				return true, nil
			}
			last = subject
			return cb(subject)
		})
	case *cayley_kv.QuadStore:
		return iterateKvSubjectsAfter(ctx, s, obj, pred, after, cb)
	}
	return errors.Errorf("graph store %T cannot page subjects", h)
}

// iterateKvSubjectsAfter walks the subjects of a Cayley kv store's reverse
// index posting, skipping postings whose quads have all been deleted.
func iterateKvSubjectsAfter(ctx context.Context, s *cayley_kv.QuadStore, obj, pred, after graph.Ref, cb func(subject graph.Ref) (bool, error)) error {
	// Find the reverse index in the open layout.
	ind, err := findCompleteIndex(s, typePageDirs)
	if err != nil {
		return err
	}

	// Walk the posting from the cursor, skipping postings with no live quad.
	var start cayley_kv.Int64Value
	if after != nil {
		start = after.(cayley_kv.Int64Value)
	}
	values := []uint64{uint64(obj.(cayley_kv.Int64Value)), uint64(pred.(cayley_kv.Int64Value))}
	err = s.IterateIndexPrefixNextRefsAfter(ctx, ind, values, start, func(ref cayley_kv.Int64Value, hasLive func() (bool, error)) error {
		// Report live postings until cb stops the walk.
		live, err := hasLive()
		if err != nil || !live {
			return err
		}
		more, err := cb(ref)
		if err == nil && !more {
			err = io.EOF
		}
		return err
	})
	if err == io.EOF {
		err = nil
	}
	return err
}

// typePageDirs are the directions of the reverse index a typed page walks:
// the type object and predicate pinned, the subjects enumerated in order.
var typePageDirs = []quad.Direction{quad.Object, quad.Predicate, quad.Subject}

// findCompleteIndex returns the complete index of h with dirs.
func findCompleteIndex(h *cayley_kv.QuadStore, dirs []quad.Direction) (cayley_kv.QuadIndex, error) {
	for _, ind := range h.Layout() {
		if ind.State == cayley_kv.QuadIndexState_COMPLETE && slices.Equal(ind.Dirs, dirs) {
			return ind, nil
		}
	}
	return cayley_kv.QuadIndex{}, errors.Errorf("graph has no complete %v index", dirs)
}
