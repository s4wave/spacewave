package refwalk

import (
	"cmp"
	"maps"
	"reflect"
	"slices"
)

// Histogram counts the blocks a walk visits by type and by stored size. Each
// block counts once: under the first type it is read as, or, when that type
// was an unconfirmed guess, under the first confirmed type a later visit
// reads. It needs a Walker that keeps bytes, since a leaf's size is read from
// its bytes.
type Histogram struct {
	// seen holds the row each counted ref is in, by the ref's key.
	seen map[string]histogramKey
	// types holds the row of each block type and certainty.
	types map[histogramKey]*HistogramRow
	// total is the row of every type.
	total HistogramRow
}

// histogramKey names a row: a block type, and whether the walk guessed it
// without confirming it.
type histogramKey struct {
	typ    reflect.Type
	unsure bool
}

// NewHistogram constructs an empty Histogram.
func NewHistogram() *Histogram {
	return &Histogram{
		seen:  make(map[string]histogramKey),
		types: make(map[histogramKey]*HistogramRow),
	}
}

// Add counts a visited block the store holds, unless it was counted before
// under a confirmed type. A guessed block's refs were never read, so it is
// no leaf.
func (h *Histogram) Add(rd Visit) {
	// Count each stored block once, a confirmed type replacing a guess.
	if !rd.Found {
		return
	}
	ref := rd.Node.Ref.MarshalString()
	rk := histogramKey{typ: reflect.TypeOf(rd.Node.Ctor()), unsure: rd.Unsure}
	size := uint64(len(rd.Data))
	if prev, ok := h.seen[ref]; ok {
		if rk.unsure || !prev.unsure {
			return
		}
		h.types[prev].remove(size, false)
		h.total.remove(size, false)
	}
	h.seen[ref] = rk

	// Count it under its row and in the total.
	row := h.types[rk]
	if row == nil {
		row = &HistogramRow{Type: rk.typ.String(), Unsure: rk.unsure}
		h.types[rk] = row
	}
	leaf := rd.Leaf || (!rk.unsure && rd.DecodeErr == nil && len(rd.Refs) == 0)
	row.add(size, leaf)
	h.total.add(size, leaf)
}

// Total returns the row of every type.
func (h *Histogram) Total() HistogramRow {
	return h.total
}

// Rows returns the row of each type, most bytes first, then by type, the
// confirmed row first.
func (h *Histogram) Rows() []HistogramRow {
	rows := make([]HistogramRow, 0, len(h.types))
	for row := range maps.Values(h.types) {
		if row.Blocks != 0 {
			rows = append(rows, *row)
		}
	}
	slices.SortFunc(rows, func(a, b HistogramRow) int {
		return cmp.Or(cmp.Compare(b.Bytes, a.Bytes), cmp.Compare(a.Type, b.Type), compareBool(a.Unsure, b.Unsure))
	})
	return rows
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}
