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
	worldWriteCommitPartSecondsDesc = prometheus.NewDesc(
		"spacewave_world_write_commit_part_seconds_total",
		"Seconds committed World writes spent in each reported part of their commit's admit and durable stages, by operation, since the process started: the engine's stages (world-state-commit, block-commit, publication-admission, advance, receipt-wait), durable-head, and the parts a transaction reports, such as sql-commit.",
		[]string{"op", "part"}, nil,
	)
	worldWriteCommitPagesDesc = prometheus.NewDesc(
		"spacewave_world_write_commit_pages_total",
		"World pages the commits of committed World writes stored, each write counting its share of its batch, by operation, since the process started.",
		[]string{"op"}, nil,
	)
	worldWriteCommitPageBytesDesc = prometheus.NewDesc(
		"spacewave_world_write_commit_page_bytes_total",
		"Length of the World pages the commits of committed World writes stored, each write counting its share of its batch, by operation, since the process started.",
		[]string{"op"}, nil,
	)
)

// writeStagesCollector reports writequeue.GetStageTotals.
type writeStagesCollector struct{}

// Describe sends the stage metric descriptors.
func (writeStagesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- worldWriteStageSecondsDesc
	ch <- worldWriteStageWritesDesc
	ch <- worldWriteCommitPartSecondsDesc
	ch <- worldWriteCommitPagesDesc
	ch <- worldWriteCommitPageBytesDesc
}

// Collect sends the stage totals as of now.
func (writeStagesCollector) Collect(ch chan<- prometheus.Metric) {
	for op, t := range writequeue.GetStageTotals() {
		ch <- prometheus.MustNewConstMetric(worldWriteStageWritesDesc, prometheus.CounterValue, float64(t.Writes), op)
		for i, secs := range t.Seconds {
			ch <- prometheus.MustNewConstMetric(worldWriteStageSecondsDesc, prometheus.CounterValue, secs, op, writequeue.Stage(i).String())
		}
		for part, secs := range t.Parts {
			ch <- prometheus.MustNewConstMetric(worldWriteCommitPartSecondsDesc, prometheus.CounterValue, secs, op, part)
		}
		ch <- prometheus.MustNewConstMetric(worldWriteCommitPagesDesc, prometheus.CounterValue, t.Pages, op)
		ch <- prometheus.MustNewConstMetric(worldWriteCommitPageBytesDesc, prometheus.CounterValue, t.PageBytes, op)
	}
}

// init registers the collector with the default registry, which the process
// metrics gather.
func init() {
	prometheus.MustRegister(writeStagesCollector{})
}

// _ is a type assertion
var _ prometheus.Collector = writeStagesCollector{}
