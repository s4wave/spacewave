//go:build !js

package sobject_world_engine

import (
	"github.com/aperturerobotics/go-prometheus-client-lite/prometheus"
	"github.com/s4wave/spacewave/db/world/writequeue"
)

// The descriptors of the World write queue's stage metrics.
var (
	worldWriteStageSecondsDesc = prometheus.NewDesc(
		"spacewave_world_write_stage_seconds_total",
		"Seconds committed World writes spent in each stage, by operation, since the process started: wait (queued to its body), body (its checks and body), batch (the bodies after it), admit (the commit's admission under the writer lock) and durable (admission to the durable result).",
		[]string{"op", "stage"}, nil,
	)
	worldWriteStageWritesDesc = prometheus.NewDesc(
		"spacewave_world_write_stage_writes_total",
		"Committed World writes the stage seconds cover, by operation, since the process started.",
		[]string{"op"}, nil,
	)
)

// writeStagesCollector reports writequeue.GetStageTotals.
type writeStagesCollector struct{}

// Describe sends the stage metric descriptors.
func (writeStagesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- worldWriteStageSecondsDesc
	ch <- worldWriteStageWritesDesc
}

// Collect sends the stage totals as of now.
func (writeStagesCollector) Collect(ch chan<- prometheus.Metric) {
	for op, t := range writequeue.GetStageTotals() {
		ch <- prometheus.MustNewConstMetric(worldWriteStageWritesDesc, prometheus.CounterValue, float64(t.Writes), op)
		for i, secs := range t.Seconds {
			ch <- prometheus.MustNewConstMetric(worldWriteStageSecondsDesc, prometheus.CounterValue, secs, op, writequeue.Stage(i).String())
		}
	}
}

// init registers the collector with the default registry, which the process
// metrics gather.
func init() {
	prometheus.MustRegister(writeStagesCollector{})
}

// _ is a type assertion
var _ prometheus.Collector = writeStagesCollector{}
