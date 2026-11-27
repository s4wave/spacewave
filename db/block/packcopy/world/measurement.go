package packcopy_world

import (
	"encoding/binary"
	"math"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// NewLookupTyper types immutable heads, resolving one object's type at a time
// through lookup instead of collecting the World's objects into memory.
// lookup must support concurrent calls when the Typer has concurrent readers.
func NewLookupTyper(le *logrus.Entry, heads []*bucket.ObjectRef, roots RootTypes, lookup func(string) (string, error)) (*Typer, error) {
	// Type the finite root set without scanning any World objects.
	t := &Typer{
		le: le, worlds: make(map[string]*nodeType), roots: roots, objectLookup: lookup,
		xfrms: make(map[string]block.Transformer), untyped: make(map[string]uint64),
	}
	for _, h := range heads {
		x, err := t.transformer(h.GetTransformConf())
		if err != nil {
			return nil, err
		}
		t.worlds[h.GetRootRef().MarshalString()] = &nodeType{ctor: world_block.NewWorldBlock, xfrm: x, xfrmConf: h.GetTransformConf()}
	}
	return t, nil
}

// MarshalType saves one type with a self-contained WalkState header and a
// WalkChunk entry. The header is length-prefixed; the remaining bytes are
// the chunk. Both messages use the generated walk-state protobuf codecs.
func (t *Typer) MarshalType(typ any) ([]byte, error) {
	// A missing descriptor stays explicitly untyped.
	if typ == nil {
		return nil, nil
	}
	s := newTypeSaver(walkRegistry(t.roots))
	wt, err := s.save(typ)
	if err != nil {
		return nil, err
	}
	if wt == nil {
		return nil, errors.New("measurement cannot persist block type")
	}

	// Save constructor names and transforms beside the one typed entry.
	h, err := (&WalkState{Ctors: s.ctorList, Xfrms: s.xfrmList}).MarshalVT()
	if err != nil {
		return nil, err
	}
	c, err := (&WalkChunk{Entries: []*WalkEntry{{Type: wt}}}).MarshalVT()
	if err != nil {
		return nil, err
	}
	return append(append(binary.AppendUvarint(nil, uint64(len(h))), h...), c...), nil
}

// UnmarshalType rebuilds a descriptor emitted by MarshalType.
func (t *Typer) UnmarshalType(data []byte) (any, error) {
	// Decode only a self-contained, length-delimited descriptor.
	if len(data) == 0 {
		return nil, nil
	}
	wide, k := binary.Uvarint(data)
	if k <= 0 || wide > math.MaxInt {
		return nil, errors.New("invalid measurement type header")
	}
	n := int(wide)
	if n > len(data)-k {
		return nil, errors.New("truncated measurement type header")
	}
	h := &WalkState{}
	if err := h.UnmarshalVT(data[k : k+n]); err != nil {
		return nil, err
	}

	// Read the one saved type after its constructor header.
	c := &WalkChunk{}
	if err := c.UnmarshalVT(data[k+n:]); err != nil {
		return nil, err
	}
	if len(c.GetEntries()) != 1 {
		return nil, errors.New("invalid measurement type entry")
	}

	// Rebuild the descriptor from the schema this binary actually registers.
	l := &typeLoader{typer: t, reg: walkRegistry(t.roots), ctors: h.GetCtors(), xfrms: h.GetXfrms(), built: make(map[uint32]xfrmPair)}
	typ, ok := l.load(c.GetEntries()[0].GetType())
	if !ok {
		return nil, errors.New("unknown measurement type")
	}
	return typ, nil
}

// TypeName names the concrete block type, preserving untyped blocks explicitly.
func (t *Typer) TypeName(typ any) string {
	if n, ok := typ.(*nodeType); ok && n != nil {
		return ctorName(n.ctor)
	}
	return "untyped"
}
