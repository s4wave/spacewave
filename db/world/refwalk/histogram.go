package refwalk

import (
	"cmp"
	"maps"
	"reflect"
	"slices"
)

// Histogram counts the blocks a walk visits by type and by stored size. Each
// block counts once: under the first type it is read as, or, when that read
// was an unconfirmed guess or a scan of a block with no type, under the first
// confirmed type a later visit reads. It needs a Walker that keeps bytes,
// since a leaf's size is read from its bytes.
type Histogram struct {
	// seen holds how each counted ref was counted, by the ref's key.
	seen map[string]histogramCount
	// types holds the row of each block type and certainty.
	types map[histogramKey]*HistogramRow
	// total is the row of every type.
	total HistogramRow
}

// histogramKey names a row: a block type, nil for the blocks a scanning walk
// read with no type, and whether the walk guessed the type without confirming
// it.
type histogramKey struct {
	typ    reflect.Type
	unsure bool
}

// confirmed reports whether the row holds blocks read as a type the walk is
// sure of.
func (k histogramKey) confirmed() bool {
	return k.typ != nil && !k.unsure
}

// name returns the row's type as HistogramRow.Type prints it.
func (k histogramKey) name() string {
	if k.typ == nil {
		return UntypedRow
	}
	return k.typ.String()
}

// UntypedRow is the HistogramRow.Type of the blocks a scanning walk read with
// no type.
const UntypedRow = "untyped"

// histogramCount is how one block was counted: its row, and whether as a
// leaf.
type histogramCount struct {
	row  histogramKey
	leaf bool
}

// NewHistogram constructs an empty Histogram.
func NewHistogram() *Histogram {
	return &Histogram{
		seen:  make(map[string]histogramCount),
		types: make(map[histogramKey]*HistogramRow),
	}
}

// Add counts a visited block the store holds, unless it was counted before
// under a confirmed type. A guessed block's refs were never read, so it is
// no leaf; a scanned block is one when the scan found no ref in it.
func (h *Histogram) Add(rd Visit) {
	// Count each stored block once, a confirmed type replacing a guess or a
	// scan.
	if !rd.Found {
		return
	}
	ref := rd.Node.Ref.MarshalString()
	rk := histogramKey{unsure: rd.Unsure}
	if rd.Node.Ctor != nil {
		rk.typ = reflect.TypeOf(rd.Node.Ctor())
	}
	size := uint64(len(rd.Data))
	if prev, ok := h.seen[ref]; ok {
		if !rk.confirmed() || prev.row.confirmed() {
			return
		}
		h.types[prev.row].remove(size, prev.leaf)
		h.total.remove(size, prev.leaf)
	}

	// Count it under its row and in the total.
	row := h.types[rk]
	if row == nil {
		row = &HistogramRow{Type: rk.name(), Unsure: rk.unsure}
		h.types[rk] = row
	}
	leaf := rd.Leaf || (!rk.unsure && rd.DecodeErr == nil && len(rd.Refs) == 0)
	h.seen[ref] = histogramCount{row: rk, leaf: leaf}
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
