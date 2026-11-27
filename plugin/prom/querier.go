package prom_plugin

import (
	"cmp"
	"context"
	"slices"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// Querier is a storage.Querier over a Queryable for one time range.
//
// Select reads only the segments that overlap the range. Label lookups ignore
// time and report the labels of every committed series that matches.
type Querier struct {
	// queryable is the Queryable the querier reads.
	queryable *Queryable
	// mint is the start of the queried range in milliseconds, inclusive.
	mint int64
	// maxt is the end of the queried range in milliseconds, inclusive.
	maxt int64
}

// NewQuerier constructs a querier for samples between mint and maxt.
func NewQuerier(q *Queryable, mint, maxt int64) *Querier {
	return &Querier{queryable: q, mint: mint, maxt: maxt}
}

// Select returns the series matching matchers, sorted by labels. Series from
// several targets with the same labels are merged. A hints range inside the
// querier's range narrows the segments read.
func (q *Querier) Select(ctx context.Context, _ bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	// Narrow the time range to the hints.
	mint, maxt := q.mint, q.maxt
	if hints != nil {
		mint, maxt = max(mint, hints.Start), min(maxt, hints.End)
	}

	// Read the matching series of every target.
	var all []storage.Series
	err := q.queryable.forEachTarget(ctx, func(r *StoreReader) error {
		series, err := r.Select(ctx, mint, maxt, matchers)
		all = append(all, series...)
		return err
	})
	if err != nil {
		return storage.ErrSeriesSet(err)
	}

	// Sort by labels and merge the series that share them.
	slices.SortFunc(all, func(a, b storage.Series) int {
		return labels.Compare(a.Labels(), b.Labels())
	})
	out := make([]storage.Series, 0, len(all))
	for len(all) != 0 {
		n := 1
		for n < len(all) && labels.Equal(all[0].Labels(), all[n].Labels()) {
			n++
		}
		if n == 1 {
			out = append(out, all[0])
		} else {
			out = append(out, storage.ChainedSeriesMerge(all[:n]...))
		}
		all = all[n:]
	}
	return NewSeriesSet(out)
}

// LabelValues returns the sorted values of label name among the series
// matching matchers.
func (q *Querier) LabelValues(ctx context.Context, name string, _ *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	// Find the series that match the selector.
	sets, err := q.matchingLabels(ctx, matchers)
	if err != nil {
		return nil, nil, err
	}

	// Collect the label's non-empty values.
	var values []string
	for _, lset := range sets {
		if v := lset.Get(name); v != "" {
			values = append(values, v)
		}
	}

	// Sort and drop repeats.
	slices.Sort(values)
	return slices.Compact(values), nil, nil
}

// LabelNames returns the sorted label names of the series matching matchers.
func (q *Querier) LabelNames(ctx context.Context, _ *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	// Find the series that match the selector.
	sets, err := q.matchingLabels(ctx, matchers)
	if err != nil {
		return nil, nil, err
	}

	// Collect every label name.
	var names []string
	for _, lset := range sets {
		lset.Range(func(l labels.Label) { names = append(names, l.Name) })
	}

	// Sort and drop repeats.
	slices.SortFunc(names, cmp.Compare)
	return slices.Compact(names), nil, nil
}

// Close releases the querier. It holds nothing.
func (q *Querier) Close() error {
	return nil
}

// matchingLabels returns the label sets of the matching series of all targets.
func (q *Querier) matchingLabels(ctx context.Context, matchers []*labels.Matcher) ([]labels.Labels, error) {
	var out []labels.Labels
	err := q.queryable.forEachTarget(ctx, func(r *StoreReader) error {
		sets, err := r.Labels(ctx, matchers)
		out = append(out, sets...)
		return err
	})
	return out, err
}

// _ is a type assertion
var _ storage.Querier = (*Querier)(nil)
