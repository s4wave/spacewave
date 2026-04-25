package resource_debugdb

import (
	"runtime"
	"slices"
	"time"

	s4wave_debugdb "github.com/s4wave/spacewave/sdk/debugdb"
)

// maxRawSamples is the threshold below which raw samples are kept.
const maxRawSamples = 1000

// yieldInterval is how many operations between Gosched yields.
const yieldInterval = 100

// MetricCollector collects per-operation timing samples for a single metric.
type MetricCollector struct {
	name    string
	unit    string
	samples []float64
	start   time.Time
}

// NewMetricCollector creates a new metric collector.
func NewMetricCollector(name, unit string) *MetricCollector {
	return &MetricCollector{name: name, unit: unit}
}

// Start begins timing an operation.
func (m *MetricCollector) Start() {
	m.start = time.Now()
}

// Stop ends timing and records the sample.
func (m *MetricCollector) Stop() {
	elapsed := time.Since(m.start)
	m.samples = append(m.samples, float64(elapsed.Microseconds())/1000.0)
}

// Record adds an explicit duration sample.
func (m *MetricCollector) Record(d time.Duration) {
	m.samples = append(m.samples, float64(d.Microseconds())/1000.0)
}

// MaybeYield calls runtime.Gosched if the sample count is a multiple of yieldInterval.
func (m *MetricCollector) MaybeYield() {
	if len(m.samples)%yieldInterval == 0 {
		runtime.Gosched()
	}
}

// Build computes aggregates and returns a BenchmarkMetric.
func (m *MetricCollector) Build() *s4wave_debugdb.BenchmarkMetric {
	n := len(m.samples)
	if n == 0 {
		return &s4wave_debugdb.BenchmarkMetric{Name: m.name, Unit: m.unit}
	}

	sorted := make([]float64, n)
	copy(sorted, m.samples)
	slices.Sort(sorted)

	total := 0.0
	for _, s := range sorted {
		total += s
	}

	metric := &s4wave_debugdb.BenchmarkMetric{
		Name:    m.name,
		Unit:    m.unit,
		Count:   uint64(n),
		TotalMs: total,
		MinMs:   sorted[0],
		P50Ms:   percentile(sorted, 0.50),
		P99Ms:   percentile(sorted, 0.99),
		MaxMs:   sorted[n-1],
	}

	if n <= maxRawSamples {
		metric.Samples = m.samples
	}

	return metric
}

// percentile returns the value at the given percentile from a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	idx := int(float64(n-1) * p)
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}

// SuiteTimer helps run a suite for a target duration.
type SuiteTimer struct {
	deadline time.Time
}

// NewSuiteTimer creates a timer for a suite's proportional share of the total duration.
func NewSuiteTimer(totalDuration time.Duration, suiteCount, suiteIndex int) *SuiteTimer {
	share := totalDuration / time.Duration(suiteCount)
	return &SuiteTimer{deadline: time.Now().Add(share)}
}

// Running returns true if the suite still has time remaining.
func (t *SuiteTimer) Running() bool {
	return time.Now().Before(t.deadline)
}
