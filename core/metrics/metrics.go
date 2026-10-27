// Package metrics gathers the metrics of the running Spacewave process.
//
// Instrumentation registers its collectors with the default registry of
// go-prometheus-client-lite. Gather adds the Go runtime collectors on first
// use, so every Spacewave process reports its memory, goroutines and GC, and
// the health of each volume it collects as spacewave_volume_* metrics.
package metrics

import (
	io_prometheus_client "github.com/aperturerobotics/go-prometheus-client-lite/client_model/go"
)

// Gather returns the metric families of the process's default registry,
// sorted by name.
func Gather() ([]*io_prometheus_client.MetricFamily, error) {
	return gather()
}
