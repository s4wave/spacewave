package promstorage

import (
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// SeriesSet is a storage.SeriesSet over series held in memory.
type SeriesSet struct {
	// series are the series in iteration order.
	series []storage.Series
	// next is the index of the series the next call to Next moves to.
	next int
}

// NewSeriesSet constructs a series set that returns series in order.
func NewSeriesSet(series []storage.Series) *SeriesSet {
	return &SeriesSet{series: series}
}

// Next advances to the next series and reports whether there is one.
func (s *SeriesSet) Next() bool {
	if s.next >= len(s.series) {
		return false
	}
	s.next++
	return true
}

// At returns the current series.
func (s *SeriesSet) At() storage.Series {
	return s.series[s.next-1]
}

// Err returns nil because the series are already read.
func (s *SeriesSet) Err() error {
	return nil
}

// Warnings returns no warnings.
func (s *SeriesSet) Warnings() annotations.Annotations {
	return nil
}

// _ is a type assertion
var _ storage.SeriesSet = (*SeriesSet)(nil)
