package block_store_nodepush

import (
	"bufio"
	"fmt"
	"io"
)

// nodeMetric is one sample of the node's metrics.
type nodeMetric struct {
	// name is the metric's name.
	name string
	// kind is "gauge" or "counter".
	kind string
	// help describes the metric.
	help string
	// label is the sample's result label, or empty for none.
	label string
	// value is the sample.
	value float64
}

// WriteMetrics writes the node's metrics to w in the Prometheus text format:
// its free disk, the bytes of its pack log and read copies, the log's packs
// still to upload, and its reads by where they were served.
func (n *Node) WriteMetrics(w io.Writer) error {
	// Gather the samples, with the free disk only when known.
	st := n.Stats()
	samples := []nodeMetric{
		{"spacewave_node_log_bytes", "gauge", "Bytes of the pack log's local packs, the open one included.", "", float64(st.Log.LocalBytes)},
		{"spacewave_node_log_pending_bytes", "gauge", "Bytes of the pack log's finalized packs not yet uploaded.", "", float64(st.Upload.PendingBytes)},
		{"spacewave_node_log_pending_packs", "gauge", "The pack log's finalized packs not yet uploaded.", "", float64(st.Upload.Pending)},
		{"spacewave_node_reports_pending", "gauge", "Uploaded packs the local store has not applied.", "", float64(st.ReportsPending)},
		{"spacewave_node_read_copies", "gauge", "Read copies of the remote's packs held.", "", float64(st.Packs.Copies)},
		{"spacewave_node_read_copy_bytes", "gauge", "Bytes of the read copies held.", "", float64(st.Packs.CopyBytes)},
		{"spacewave_node_reads_total", "counter", "Range reads by how the node answered: copy from a read copy, remote by a range read of the remote, miss with nothing.", "copy", float64(st.Packs.CopyReads)},
		{"spacewave_node_reads_total", "counter", "", "remote", float64(st.Packs.RemoteReads)},
		{"spacewave_node_reads_total", "counter", "", "miss", float64(st.Packs.Misses)},
		{"spacewave_node_copies_total", "counter", "Read copies stored by how they came: filled from the remote, or kept after an upload; failed counts failed fills.", "filled", float64(st.Packs.Filled)},
		{"spacewave_node_copies_total", "counter", "", "kept", float64(st.Packs.Kept)},
		{"spacewave_node_copies_total", "counter", "", "failed", float64(st.Packs.FillsFailed)},
		{"spacewave_node_filled_bytes_total", "counter", "Bytes of the packs filled from the remote.", "", float64(st.Packs.FilledBytes)},
		{"spacewave_node_pruned_bytes_total", "counter", "Bytes of the read copies pruned for disk space.", "", float64(st.Packs.PrunedBytes)},
		{"spacewave_node_backfill_state", "gauge", "The backfill: 0 off, 1 running, 2 switched on and stopped.", "", float64(st.Backfill)},
	}
	if st.FreeKnown {
		samples = append(samples, nodeMetric{"spacewave_node_free_bytes", "gauge", "Free disk of the node's directory.", "", float64(st.FreeBytes)})
	}

	// Write each family's header once, before its first sample.
	bw := bufio.NewWriter(w)
	for _, m := range samples {
		if m.help != "" {
			fmt.Fprintf(bw, "# HELP %s %s\n# TYPE %s %s\n", m.name, m.help, m.name, m.kind)
		}
		if m.label != "" {
			fmt.Fprintf(bw, "%s{result=%q} %g\n", m.name, m.label, m.value)
			continue
		}
		fmt.Fprintf(bw, "%s %g\n", m.name, m.value)
	}
	return bw.Flush()
}
