package prom_plugin

import (
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// referenceSample is a float sample of the in-memory reference storage.
type referenceSample struct {
	// t is the sample timestamp in milliseconds.
	t int64
	// f is the sample value.
	f float64
}

// T returns the sample timestamp.
func (s referenceSample) T() int64 { return s.t }

// ST returns zero because the reference samples carry no start timestamp.
func (s referenceSample) ST() int64 { return 0 }

// F returns the sample value.
func (s referenceSample) F() float64 { return s.f }

// H returns nil because the sample is a float.
func (s referenceSample) H() *histogram.Histogram { return nil }

// FH returns nil because the sample is a float.
func (s referenceSample) FH() *histogram.FloatHistogram { return nil }

// Type returns the float value type.
func (s referenceSample) Type() chunkenc.ValueType { return chunkenc.ValFloat }

// Copy returns the sample, which holds no references.
func (s referenceSample) Copy() chunks.Sample { return s }

// _ is a type assertion
var _ chunks.Sample = referenceSample{}
