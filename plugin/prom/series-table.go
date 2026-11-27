package prom_plugin

import (
	"github.com/prometheus/prometheus/model/labels"
)

// SeriesTable maps the label sets of a target's series to dense series
// numbers. A number is the index of the label set in the table, is never
// reused, and is what segments store in place of the labels.
//
// A SeriesTable is not safe for concurrent use.
type SeriesTable struct {
	// byKey maps the encoded label set to its series number.
	byKey map[string]uint64
	// sets holds the label sets in series number order.
	sets []labels.Labels
}

// NewSeriesTable constructs an empty series table.
func NewSeriesTable() *SeriesTable {
	return &SeriesTable{byKey: make(map[string]uint64)}
}

// Len returns the number of series in the table.
func (t *SeriesTable) Len() uint64 {
	return uint64(len(t.sets))
}

// Lookup returns the series number of lset.
func (t *SeriesTable) Lookup(lset labels.Labels) (uint64, bool) {
	num, ok := t.byKey[string(lset.Bytes(nil))]
	return num, ok
}

// Add appends lset as the next series and returns its number.
// The caller checks Lookup first. Add does not look for duplicates.
func (t *SeriesTable) Add(lset labels.Labels) uint64 {
	num := t.Len()
	t.byKey[string(lset.Bytes(nil))] = num
	t.sets = append(t.sets, lset)
	return num
}

// Labels returns the label set of series num.
func (t *SeriesTable) Labels(num uint64) labels.Labels {
	return t.sets[num]
}

// LabelsFrom returns the label sets from series number from to the end.
func (t *SeriesTable) LabelsFrom(from uint64) []labels.Labels {
	return t.sets[from:]
}

// Truncate drops the series numbered n and above.
func (t *SeriesTable) Truncate(n uint64) {
	for _, lset := range t.sets[n:] {
		delete(t.byKey, string(lset.Bytes(nil)))
	}
	t.sets = t.sets[:n]
}
