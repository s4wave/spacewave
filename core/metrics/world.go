//go:build !js

package metrics

import (
	"github.com/aperturerobotics/go-prometheus-client-lite/prometheus"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
)

// World commit metric descriptors.
var (
	worldCommitPublicationsDesc = prometheus.NewDesc(
		"spacewave_world_commit_publications_total",
		"World commits whose publication the store admitted, since the process started.",
		nil, nil,
	)
	worldCommitBlocksDesc = prometheus.NewDesc(
		"spacewave_world_commit_blocks_total",
		"Blocks the admitted World commit publications carried, by kind: world pages, payload or replay, since the process started.",
		[]string{"kind"}, nil,
	)
	worldCommitBytesDesc = prometheus.NewDesc(
		"spacewave_world_commit_bytes_total",
		"Length of the records the admitted World commit publications wrote, by kind: world pages, payload, replay or state, since the process started.",
		[]string{"kind"}, nil,
	)
	worldCheckpointsDesc = prometheus.NewDesc(
		"spacewave_world_checkpoints_total",
		"Stable World checkpoints prepared, by outcome: written, superseded, stale or failed, since the process started.",
		[]string{"outcome"}, nil,
	)
	worldCheckpointSkipsDesc = prometheus.NewDesc(
		"spacewave_world_checkpoint_skips_total",
		"Stable points the World engines did not checkpoint, by reason, since the process started.",
		[]string{"reason"}, nil,
	)
)

// worldCollector reports what the process's World engines stored in their
// commits and how their checkpoints went.
type worldCollector struct{}

// Describe sends the World commit metric descriptors.
func (worldCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		worldCommitPublicationsDesc, worldCommitBlocksDesc, worldCommitBytesDesc,
		worldCheckpointsDesc, worldCheckpointSkipsDesc,
	} {
		ch <- d
	}
}

// Collect sends the process's commit flows as of now.
func (worldCollector) Collect(ch chan<- prometheus.Metric) {
	// Report the publications and their records by kind. The state is not a
	// block, so it reports bytes only.
	f := sobject_world_engine.GetCommitFlows()
	ch <- prometheus.MustNewConstMetric(worldCommitPublicationsDesc, prometheus.CounterValue, float64(f.Publications))
	for _, k := range f.Kinds {
		if k.Kind != "state" {
			ch <- prometheus.MustNewConstMetric(worldCommitBlocksDesc, prometheus.CounterValue, float64(k.Blocks), k.Kind)
		}
		ch <- prometheus.MustNewConstMetric(worldCommitBytesDesc, prometheus.CounterValue, float64(k.Bytes), k.Kind)
	}

	// Report the checkpoints by outcome and the skips by reason.
	for _, c := range f.Checkpoints {
		ch <- prometheus.MustNewConstMetric(worldCheckpointsDesc, prometheus.CounterValue, float64(c.Count), c.Name)
	}
	for _, c := range f.Skips {
		ch <- prometheus.MustNewConstMetric(worldCheckpointSkipsDesc, prometheus.CounterValue, float64(c.Count), c.Name)
	}
}
