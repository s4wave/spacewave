package packcopy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// lineTyper types blocks whose bytes list their children, one "type ref" line
// after the first. Type "node" decodes, type "leaf" holds no refs, and any
// other type does not decode.
type lineTyper struct {
	// root is the root's type.
	root string
}

// RootType returns the root's type.
func (l lineTyper) RootType(*block.BlockRef) any { return l.root }

// Decode reads the child lines of a "node" block.
func (lineTyper) Decode(_ context.Context, typ any, _ *block.BlockRef, data []byte) ([]TypedRef, bool, error) {
	switch typ {
	case "leaf":
		return nil, true, nil
	case "node":
	default:
		return nil, false, nil
	}
	lines := strings.Split(string(data), "\n")
	var out []TypedRef
	for _, line := range lines[1:] {
		kind, b58, _ := strings.Cut(line, " ")
		ref, err := block.UnmarshalBlockRefB58(b58)
		if err != nil {
			return nil, false, err
		}
		var t any
		if kind != "-" {
			t = kind
		}
		out = append(out, TypedRef{Ref: ref, Type: t})
	}
	return out, true, nil
}

// mapSource serves stored blocks from a map.
type mapSource map[string]*block.StoredBlock

// GetStoredBlock returns the block, or nil.
func (m mapSource) GetStoredBlock(_ context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return m[ref.MarshalString()], nil
}

// typedBlock is a test block whose bytes name its typed children.
type typedBlock struct {
	ref  *block.BlockRef
	data []byte
	refs []*block.BlockRef
}

// newTypedBlock builds a block naming each child with its type, "-" for none.
func newTypedBlock(t *testing.T, name string, kids ...any) *typedBlock {
	t.Helper()
	b := &typedBlock{}
	text := name
	for i := 0; i < len(kids); i += 2 {
		child := kids[i+1].(*typedBlock)
		text += "\n" + kids[i].(string) + " " + child.ref.MarshalString()
		b.refs = append(b.refs, child.ref)
	}
	b.data = []byte(text)
	ref, err := block.BuildBlockRef(b.data, nil)
	if err != nil {
		t.Fatal(err)
	}
	b.ref = ref
	return b
}

// TestCopyTyped copies blocks whose source lacks their refs by decoding them
// by type: a typed block yields its refs and its children's types, a type
// that does not decode leaves a hole, and an untyped block keeps the old hole.
func TestCopyTyped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Build the graph. Only root's refs are known to the source.
	leaf := newTypedBlock(t, "leaf")
	deep := newTypedBlock(t, "deep", "leaf", leaf)
	typed := newTypedBlock(t, "typed", "node", deep)
	opaque := newTypedBlock(t, "opaque", "leaf", leaf)
	untyped := newTypedBlock(t, "untyped")
	root := newTypedBlock(t, "root", "node", typed, "blob", opaque, "-", untyped)
	src := mapSource{}
	for _, b := range []*typedBlock{leaf, deep, typed, opaque, untyped} {
		src[b.ref.MarshalString()] = &block.StoredBlock{Data: b.data}
	}
	src[root.ref.MarshalString()] = &block.StoredBlock{Data: root.data, Refs: root.refs, RefsKnown: true}
	// deep's store knows its refs, but its leaf child is typed only by deep.
	src[deep.ref.MarshalString()] = &block.StoredBlock{Data: deep.data, Refs: deep.refs, RefsKnown: true}

	// Copy with the typer.
	_, client := s3test.New(t)
	target := block_store_s3.NewPackStore(le, client, s3test.BucketName, "v2/", &block_store_s3.PackStoreOptions{KeepPacks: true, Index: store_kvtx_inmem.NewStore()})
	defer target.Close()
	holes := &holeList{reasons: make(map[string]string)}
	result, err := Copy(ctx, &Options{
		Roots:   []*block.BlockRef{root.ref},
		Target:  target,
		Sources: []Source{src},
		Proofs:  store_kvtx_inmem.NewStore(),
		Holes:   holes.add,
		Typer:   lineTyper{root: "node"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The typed subtree is copied with its decoded refs; the others are holes.
	want := map[string]string{
		opaque.ref.MarshalString():  ReasonUndecodable,
		untyped.ref.MarshalString(): ReasonRefsUnknown,
	}
	if len(holes.reasons) != len(want) {
		t.Fatalf("holes %v, want %v", holes.reasons, want)
	}
	for k, reason := range want {
		if holes.reasons[k] != reason {
			t.Fatalf("hole %s is %q, want %q", k, holes.reasons[k], reason)
		}
	}
	if result.Complete || result.Read != 4 {
		t.Fatalf("result %+v, want 4 read and incomplete", result)
	}
	stored, err := target.GetStoredBlock(ctx, typed.ref)
	if err != nil || stored == nil || !stored.RefsKnown || len(stored.Refs) != 1 || !stored.Refs[0].EqualsRef(deep.ref) {
		t.Fatalf("typed block stored as %v, %v", stored, err)
	}
}
