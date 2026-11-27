package prom_plugin

import (
	"context"
	"math"
	"strconv"

	io_prometheus_client "github.com/aperturerobotics/go-prometheus-client-lite/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

// gatherFunc gathers the metric families of a process.
type gatherFunc func(ctx context.Context) ([]*io_prometheus_client.MetricFamily, error)

// gatherSource returns the source that converts the families gather returns
// into the samples the text exposition format would carry.
func gatherSource(gather gatherFunc) scrapeSource {
	return func(ctx context.Context) ([]scrapeSample, error) {
		// Gather every family before converting any of them.
		families, err := gather(ctx)
		if err != nil {
			return nil, err
		}

		// Convert each family in turn.
		var out []scrapeSample
		for _, mf := range families {
			out = familySamples(out, mf)
		}
		return out, nil
	}
}

// familySamples appends the float samples of mf to out. A summary adds its
// quantiles, _sum and _count. A histogram adds its cumulative _bucket series,
// including +Inf, with _sum and _count. Native histogram buckets are skipped.
func familySamples(out []scrapeSample, mf *io_prometheus_client.MetricFamily) []scrapeSample {
	name := mf.GetName()
	for _, m := range mf.GetMetric() {
		// add appends one sample of m named name+suffix with an optional extra label.
		add := func(suffix, extraName, extraValue string, v float64) {
			// Build the sorted label set.
			b := labels.NewScratchBuilder(len(m.GetLabel()) + 2)
			b.Add(model.MetricNameLabel, name+suffix)
			for _, lp := range m.GetLabel() {
				b.Add(lp.GetName(), lp.GetValue())
			}
			if extraName != "" {
				b.Add(extraName, extraValue)
			}
			b.Sort()

			// Append the sample under it.
			out = append(out, scrapeSample{lset: b.Labels(), v: v})
		}

		// Expose the metric as its type does in the text format.
		switch mf.GetType() {
		case io_prometheus_client.MetricType_COUNTER:
			add("", "", "", m.GetCounter().GetValue())
		case io_prometheus_client.MetricType_GAUGE:
			add("", "", "", m.GetGauge().GetValue())
		case io_prometheus_client.MetricType_UNTYPED:
			add("", "", "", m.GetUntyped().GetValue())
		case io_prometheus_client.MetricType_SUMMARY:
			s := m.GetSummary()
			for _, q := range s.GetQuantile() {
				add("", model.QuantileLabel, formatFloat(q.GetQuantile()), q.GetValue())
			}
			add("_sum", "", "", s.GetSampleSum())
			add("_count", "", "", float64(s.GetSampleCount()))
		case io_prometheus_client.MetricType_HISTOGRAM, io_prometheus_client.MetricType_GAUGE_HISTOGRAM:
			h := m.GetHistogram()
			count := float64(h.GetSampleCount())
			if h.SampleCountFloat != nil {
				count = h.GetSampleCountFloat()
			}
			inf := false
			for _, bk := range h.GetBucket() {
				v := float64(bk.GetCumulativeCount())
				if bk.CumulativeCountFloat != nil {
					v = bk.GetCumulativeCountFloat()
				}
				inf = inf || math.IsInf(bk.GetUpperBound(), 1)
				add("_bucket", model.BucketLabel, formatFloat(bk.GetUpperBound()), v)
			}
			if !inf {
				add("_bucket", model.BucketLabel, "+Inf", count)
			}
			add("_sum", "", "", h.GetSampleSum())
			add("_count", "", "", count)
		}
	}
	return out
}

// formatFloat formats a quantile or bucket bound as the text format does.
func formatFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "+Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	case math.IsNaN(f):
		return "NaN"
	default:
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
}
