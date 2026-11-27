package promstorage

import (
	"github.com/prometheus/prometheus/model/labels"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
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

// NewLabelSet converts a Prometheus label set to its stored form.
func NewLabelSet(lset labels.Labels) *s4wave_prom.LabelSet {
	out := &s4wave_prom.LabelSet{Labels: make([]*s4wave_prom.Label, 0, lset.Len())}
	lset.Range(func(l labels.Label) {
		out.Labels = append(out.Labels, &s4wave_prom.Label{Name: l.Name, Value: l.Value})
	})
	return out
}

// ToLabels converts a stored label set to a Prometheus label set.
func ToLabels(s *s4wave_prom.LabelSet) labels.Labels {
	b := labels.NewScratchBuilder(len(s.GetLabels()))
	for _, l := range s.GetLabels() {
		b.Add(l.GetName(), l.GetValue())
	}
	b.Sort()
	return b.Labels()
}

// mergeLabels returns the union of the target's labels and a series' labels.
// The two sets share no names, which Store.Append enforces.
func mergeLabels(target, series labels.Labels) labels.Labels {
	// Collect both sets into one builder.
	b := labels.NewScratchBuilder(target.Len() + series.Len())
	target.Range(func(l labels.Label) { b.Add(l.Name, l.Value) })
	series.Range(func(l labels.Label) { b.Add(l.Name, l.Value) })

	// Sort by name, as a label set requires.
	b.Sort()
	return b.Labels()
}
