//go:build !js

package sobject_world_engine

import "github.com/aperturerobotics/go-prometheus-client-lite/prometheus"

// The descriptors of the World write metrics.
var (
	worldCommitsDesc = prometheus.NewDesc(
		"spacewave_world_commits_total",
		"World commits published atomically, since the process started.",
		nil, nil,
	)
	worldCommitBytesDesc = prometheus.NewDesc(
		"spacewave_world_commit_bytes_total",
		"Length the published World commits stored, by kind, since the process started.",
		[]string{"kind"}, nil,
	)
	worldCheckpointsDesc = prometheus.NewDesc(
		"spacewave_world_checkpoints_total",
		"Stable checkpoints the World engines adopted, since the process started.",
		nil, nil,
	)
	worldCheckpointSkipsDesc = prometheus.NewDesc(
		"spacewave_world_checkpoint_skips_total",
		"Due stable checkpoints the World engines skipped, by reason, since the process started.",
		[]string{"reason"}, nil,
	)
	worldDeadlistsUnrecordedDesc = prometheus.NewDesc(
		"spacewave_world_deadlists_unrecorded_total",
		"Pending deadlist entries naming no record at open, forfeited or cut off by a restart, since the process started.",
		nil, nil,
	)
)

// writeFlowsCollector reports GetWriteFlows.
type writeFlowsCollector struct{}

// Describe sends the World write metric descriptors.
func (writeFlowsCollector) Describe(ch chan<- *prometheus.Desc) {
	// Send the write, checkpoint and deadlist descriptors.
	ch <- worldCommitsDesc
	ch <- worldCommitBytesDesc
	ch <- worldCheckpointsDesc
	ch <- worldCheckpointSkipsDesc
	ch <- worldDeadlistsUnrecordedDesc
}

// Collect sends the counters as of now.
func (writeFlowsCollector) Collect(ch chan<- prometheus.Metric) {
	// Report the commits and their bytes, then the checkpoints.
	f := GetWriteFlows()
	ch <- prometheus.MustNewConstMetric(worldCommitsDesc, prometheus.CounterValue, float64(f.Commits))
	for k, n := range f.Bytes {
		ch <- prometheus.MustNewConstMetric(worldCommitBytesDesc, prometheus.CounterValue, float64(n), WriteKind(k).String())
	}
	ch <- prometheus.MustNewConstMetric(worldCheckpointsDesc, prometheus.CounterValue, float64(f.Checkpoints))
	for s, n := range f.Skips {
		ch <- prometheus.MustNewConstMetric(worldCheckpointSkipsDesc, prometheus.CounterValue, float64(n), CheckpointSkip(s).String())
	}

	// Report the unrecorded deadlist entries.
	ch <- prometheus.MustNewConstMetric(worldDeadlistsUnrecordedDesc, prometheus.CounterValue, float64(f.Unrecorded))
}

// init registers the collector with the default registry, which the process
// metrics gather.
func init() {
	prometheus.MustRegister(writeFlowsCollector{})
}

// _ is a type assertion
var _ prometheus.Collector = writeFlowsCollector{}
