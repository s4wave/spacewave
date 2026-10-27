//go:build !js

package metrics

import (
	"strings"
	"time"

	"github.com/aperturerobotics/go-prometheus-client-lite/prometheus"
	"github.com/s4wave/spacewave/db/volume"
)

// volumeLabel names the volume of a volume metric.
const volumeLabel = "volume"

// The descriptors of the volume health metrics.
var (
	volumeGCPhaseDesc = prometheus.NewDesc(
		"spacewave_volume_gc_phase",
		"Whether the volume's block collection is in the phase: 1 for the current phase, 0 for the others.",
		[]string{volumeLabel, "phase"}, nil,
	)
	volumeGCPhaseSecondsDesc = prometheus.NewDesc(
		"spacewave_volume_gc_phase_seconds",
		"Seconds the volume has been in its current collection phase.",
		[]string{volumeLabel}, nil,
	)
	volumeFileBytesDesc = prometheus.NewDesc(
		"spacewave_volume_file_bytes",
		"Length of the volume's storage files, as last measured.",
		[]string{volumeLabel}, nil,
	)
	volumeLiveValueBytesDesc = prometheus.NewDesc(
		"spacewave_volume_live_value_bytes",
		"Length of the live values the volume's storage holds, as last measured.",
		[]string{volumeLabel}, nil,
	)
	volumeFreeDiskBytesDesc = prometheus.NewDesc(
		"spacewave_volume_free_disk_bytes",
		"Disk space free to the process on the volume's file system, as last measured.",
		[]string{volumeLabel}, nil,
	)
	volumeSizesAgeSecondsDesc = prometheus.NewDesc(
		"spacewave_volume_sizes_age_seconds",
		"Seconds since the volume's sizes were measured.",
		[]string{volumeLabel}, nil,
	)
	volumePublicationPendingDesc = prometheus.NewDesc(
		"spacewave_volume_publication_pending",
		"Publications queued for the volume's writer.",
		[]string{volumeLabel}, nil,
	)
	volumePublicationPendingBytesDesc = prometheus.NewDesc(
		"spacewave_volume_publication_pending_bytes",
		"Admission cost of the publications queued for the volume's writer.",
		[]string{volumeLabel}, nil,
	)
	volumeWriteReserveDesc = prometheus.NewDesc(
		"spacewave_volume_write_reserve",
		"Whether the volume refuses new writes because free disk is below its reserve.",
		[]string{volumeLabel}, nil,
	)
)

// volumeCollector reports the health of each volume the process collects.
type volumeCollector struct{}

// Describe sends the volume metric descriptors.
func (volumeCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		volumeGCPhaseDesc, volumeGCPhaseSecondsDesc, volumeFileBytesDesc,
		volumeLiveValueBytesDesc, volumeFreeDiskBytesDesc, volumeSizesAgeSecondsDesc,
		volumePublicationPendingDesc, volumePublicationPendingBytesDesc, volumeWriteReserveDesc,
	} {
		ch <- d
	}
}

// Collect sends each registered volume's health as of now.
func (volumeCollector) Collect(ch chan<- prometheus.Metric) {
	now := time.Now()
	for _, h := range volume.HealthSnapshots() {
		collectVolume(ch, h, now)
	}
}

// collectVolume sends the metrics of one volume's health.
func collectVolume(ch chan<- prometheus.Metric, h *volume.VolumeHealth, now time.Time) {
	// Report the phase, one series for each named phase.
	id := h.GetVolumeId()
	for phase := volume.VolumeGCPhase_VOLUME_GC_PHASE_IDLE; phase <= volume.VolumeGCPhase_VOLUME_GC_PHASE_MAINTENANCE; phase++ {
		ch <- prometheus.MustNewConstMetric(volumeGCPhaseDesc, prometheus.GaugeValue, boolValue(h.GetGcPhase() == phase), id, gcPhaseName(phase))
	}
	ch <- prometheus.MustNewConstMetric(volumeGCPhaseSecondsDesc, prometheus.GaugeValue, now.Sub(h.GetGcPhaseStart().AsTime()).Seconds(), id)

	// Report the queue and the write state.
	ch <- prometheus.MustNewConstMetric(volumePublicationPendingDesc, prometheus.GaugeValue, float64(h.GetPublicationPending()), id)
	ch <- prometheus.MustNewConstMetric(volumePublicationPendingBytesDesc, prometheus.GaugeValue, float64(h.GetPublicationPendingBytes()), id)
	reserve := h.GetWriteState() == volume.VolumeWriteState_VOLUME_WRITE_STATE_RESERVE
	ch <- prometheus.MustNewConstMetric(volumeWriteReserveDesc, prometheus.GaugeValue, boolValue(reserve), id)

	// Report the sizes once the storage measured them.
	if !h.GetSizesKnown() {
		return
	}
	ch <- prometheus.MustNewConstMetric(volumeFileBytesDesc, prometheus.GaugeValue, float64(h.GetFileBytes()), id)
	ch <- prometheus.MustNewConstMetric(volumeLiveValueBytesDesc, prometheus.GaugeValue, float64(h.GetLiveValueBytes()), id)
	ch <- prometheus.MustNewConstMetric(volumeFreeDiskBytesDesc, prometheus.GaugeValue, float64(h.GetFreeDiskBytes()), id)
	ch <- prometheus.MustNewConstMetric(volumeSizesAgeSecondsDesc, prometheus.GaugeValue, now.Sub(h.GetSizesAt().AsTime()).Seconds(), id)
}

// gcPhaseName returns the phase label of phase, such as "mark".
func gcPhaseName(phase volume.VolumeGCPhase) string {
	return strings.ToLower(strings.TrimPrefix(phase.String(), "VOLUME_GC_PHASE_"))
}

// boolValue returns 1 for true and 0 for false.
func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// _ is a type assertion
var _ prometheus.Collector = volumeCollector{}
