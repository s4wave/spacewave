//go:build !js

package metrics

import (
	"strconv"

	"github.com/aperturerobotics/go-prometheus-client-lite/prometheus"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// The descriptors of the World page metrics.
var (
	worldStoredPagesDesc = prometheus.NewDesc(
		"spacewave_world_stored_pages_total",
		"World pages the process's Spaces stored, by the part of the World that holds them and their level below the World block, since it started.",
		[]string{"tree", "level"}, nil,
	)
	worldStoredPageBytesDesc = prometheus.NewDesc(
		"spacewave_world_stored_page_bytes_total",
		"Stored length of those pages, by tree and level, since the process started.",
		[]string{"tree", "level"}, nil,
	)
)

// worldCollector reports the World pages the process stored.
type worldCollector struct{}

// Describe sends the World page metric descriptors.
func (worldCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- worldStoredPagesDesc
	ch <- worldStoredPageBytesDesc
}

// Collect sends the stored page counts as of now.
func (worldCollector) Collect(ch chan<- prometheus.Metric) {
	for key, n := range world_block.StoredPages.Snapshot() {
		tree, level := key.Tree.String(), strconv.Itoa(key.Level)
		ch <- prometheus.MustNewConstMetric(worldStoredPagesDesc, prometheus.CounterValue, float64(n.Pages), tree, level)
		ch <- prometheus.MustNewConstMetric(worldStoredPageBytesDesc, prometheus.CounterValue, float64(n.Bytes), tree, level)
	}
}
