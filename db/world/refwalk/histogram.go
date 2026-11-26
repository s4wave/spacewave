package refwalk

import (
	"cmp"
	"maps"
	"reflect"
	"slices"
)

// Histogram counts the blocks a walk visits by type and by stored size. Each
// block counts once, under the first type it is read as. It needs a Walker
// that keeps bytes, since a leaf's size is read from its bytes.
type Histogram struct {
	// seen holds the refs counted, by their keys.
	seen map[string]struct{}
	// types holds the row of each block type.
	types map[reflect.Type]*HistogramRow
	// total is the row of every type.
	total HistogramRow
}

// NewHistogram constructs an empty Histogram.
func NewHistogram() *Histogram {
	return &Histogram{
		seen:  make(map[string]struct{}),
		types: make(map[reflect.Type]*HistogramRow),
	}
}

// Add counts a visited block the store holds, unless it was counted before.
func (h *Histogram) Add(rd Visit) {
	// Count each stored block once.
	if !rd.Found {
		return
	}
	key := rd.Node.Ref.MarshalString()
	if _, ok := h.seen[key]; ok {
		return
	}
	h.seen[key] = struct{}{}

	// Count it under its type and in the total.
	typ := reflect.TypeOf(rd.Node.Ctor())
	row := h.types[typ]
	if row == nil {
		row = &HistogramRow{Type: typ.String()}
		h.types[typ] = row
	}
	leaf := rd.Leaf || (rd.DecodeErr == nil && len(rd.Refs) == 0)
	size := uint64(len(rd.Data))
	row.add(size, leaf)
	h.total.add(size, leaf)
}

// Total returns the row of every type.
func (h *Histogram) Total() HistogramRow {
	return h.total
}

// Rows returns the row of each type, most bytes first, then by type.
func (h *Histogram) Rows() []HistogramRow {
	rows := make([]HistogramRow, 0, len(h.types))
	for row := range maps.Values(h.types) {
		rows = append(rows, *row)
	}
	slices.SortFunc(rows, func(a, b HistogramRow) int {
		return cmp.Or(cmp.Compare(b.Bytes, a.Bytes), cmp.Compare(a.Type, b.Type))
	})
	return rows
}
