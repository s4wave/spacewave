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

// SubjectPager is a quad store that lists the distinct subjects of an object
// and predicate in a stable order, resuming after a subject.
type SubjectPager interface {
	graph.QuadStore
	// IterateSubjectsAfter calls cb with each distinct subject of the quads
	// with obj and pred, in the store's order, after the subject after, or
	// from the first when after is nil. An error from cb stops the walk and
	// is returned.
	IterateSubjectsAfter(ctx context.Context, obj, pred, after graph.Ref, cb func(subject graph.Ref) error) error
}

// NewSubjectPager returns the pager of qs, or nil when qs cannot page.
func NewSubjectPager(qs graph.QuadStore) SubjectPager {
	switch s := qs.(type) {
	case SubjectPager:
		return s
	case *cayley_kv.QuadStore:
		return kvSubjectPager{s}
	}
	return nil
}

// ListObjectsWithTypeOnGraph returns distinct subjects in graph-index order.
// startAfter is the previous page's last key; empty begins the listing.
// limit must be non-zero and is capped at ObjectTypePageLimit. The World must
// remain unchanged to retain exactly-once traversal across pages.
func ListObjectsWithTypeOnGraph(ctx context.Context, h SubjectPager, typeID, startAfter string, limit uint32) ([]string, bool, error) {
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
	err = h.IterateSubjectsAfter(ctx, typeRef, predRef, after, func(ref graph.Ref) error {
		// Detect a following page without decoding its first subject.
		if len(keys) == int(limit) {
			more = true
			return io.EOF
		}

		// Decode the subject from its node reference.
		value, err := h.NameOf(ctx, ref)
		if err != nil {
			return err
		}
		key, ok := value.(quad.IRI)
		if !ok {
			return errors.New("type subject is not an object key")
		}
		keys = append(keys, string(key))
		return nil
	})
	if err == io.EOF {
		err = nil
	}
	return keys, more, err
}

// kvSubjectPager pages the subjects of a Cayley kv store through its reverse
// index, skipping postings whose quads have all been deleted.
type kvSubjectPager struct {
	*cayley_kv.QuadStore
}

// IterateSubjectsAfter walks the subjects of the reverse index's posting.
func (s kvSubjectPager) IterateSubjectsAfter(ctx context.Context, obj, pred, after graph.Ref, cb func(subject graph.Ref) error) error {
	// Find the reverse index in the open layout.
	ind, err := findCompleteIndex(s.QuadStore, typePageDirs)
	if err != nil {
		return err
	}

	// Walk the posting from the cursor, skipping postings with no live quad.
	var start cayley_kv.Int64Value
	if after != nil {
		start = after.(cayley_kv.Int64Value)
	}
	values := []uint64{uint64(obj.(cayley_kv.Int64Value)), uint64(pred.(cayley_kv.Int64Value))}
	return s.IterateIndexPrefixNextRefsAfter(ctx, ind, values, start, func(ref cayley_kv.Int64Value, hasLive func() (bool, error)) error {
		live, err := hasLive()
		if err != nil || !live {
			return err
		}
		return cb(ref)
	})
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
