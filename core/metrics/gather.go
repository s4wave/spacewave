//go:build !js

package metrics

import (
	"sync"

	io_prometheus_client "github.com/aperturerobotics/go-prometheus-client-lite/client_model/go"
	"github.com/aperturerobotics/go-prometheus-client-lite/prometheus"
)

// registerRuntime registers the Go runtime, build info and volume health
// collectors once.
var registerRuntime = sync.OnceValue(func() error {
	if err := prometheus.Register(prometheus.NewGoCollector()); err != nil {
		return err
	}
	if err := prometheus.Register(volumeCollector{}); err != nil {
		return err
	}
	return prometheus.Register(prometheus.NewBuildInfoCollector())
})

// gather gathers the default registry with the runtime collectors.
func gather() ([]*io_prometheus_client.MetricFamily, error) {
	if err := registerRuntime(); err != nil {
		return nil, err
	}
	return prometheus.DefaultGatherer.Gather()
}
