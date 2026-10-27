package kvtx

import (
	"context"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/volume"
)

// HealthSizesFn measures a volume's storage: the length of its files, the
// length of the live values they hold, and the disk space free to the
// process.
type HealthSizesFn func(ctx context.Context) (fileBytes, liveValueBytes, freeDiskBytes uint64, err error)

// healthPoll is how often the size measurement checks whether it is due.
const healthPoll = time.Second

// healthMaxAge is the age past which measured sizes are measured again,
// however little the volume wrote, since other processes use the disk too.
const healthMaxAge = 30 * time.Second

// healthHeadroomDivisor sets how much of the free disk the volume's own writes
// may use before the sizes are measured again: one part in this many.
const healthHeadroomDivisor = 8

// healthMinWrite is the least the volume writes between measurements, so a
// nearly full disk is not measured on every poll.
const healthMinWrite = 1 << 20

// healthState is the collection phase and the measured sizes of a volume,
// guarded by the volume's healthBcast.
type healthState struct {
	// phase is the collection step the volume is in.
	phase volume.VolumeGCPhase
	// phaseStart is when the volume entered phase.
	phaseStart time.Time
	// sizesAt is when the sizes were measured, zero before the first.
	sizesAt time.Time
	// fileBytes, liveValueBytes and freeDiskBytes are the measured sizes.
	fileBytes, liveValueBytes, freeDiskBytes uint64
}

// SetHealthSizes starts measuring the storage with fn, which the health
// snapshot reports. It measures once at once, then when the volume's committed
// writes pass a fraction of the free disk, when a collection may have freed
// space, and at least every 30 seconds. It is called once, before the volume
// is shared, and Close stops it.
func (v *Volume) SetHealthSizes(fn HealthSizesFn) {
	// Start the measurement under a context Close cancels.
	ctx, cancel := context.WithCancel(context.Background())
	v.healthSizes = fn
	v.healthWake = make(chan struct{}, 1)
	v.healthCancel = cancel
	v.healthDone = make(chan struct{})
	go v.runHealthSizes(ctx)
}

// GetHealthSnapshotWithWait returns the volume's health and a channel that
// closes when its collection phase or measured sizes change. The publication
// occupancy is read with the snapshot; its changes alone do not close the
// channel, so a write wakes no watcher.
func (v *Volume) GetHealthSnapshotWithWait() (*volume.VolumeHealth, <-chan struct{}) {
	// Copy the state and subscribe to its next change.
	var st healthState
	var waitCh <-chan struct{}
	v.healthBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		st = v.health
		waitCh = getWaitCh()
	})

	// Build the snapshot with the current publication occupancy.
	pub := v.GetPublicationStats()
	h := &volume.VolumeHealth{
		VolumeId:                v.GetID(),
		GcPhase:                 st.phase,
		GcPhaseStart:            timestamppb.New(st.phaseStart),
		PublicationPending:      uint32(pub.Pending),      // #nosec G115 -- the queue is bounded far below 2^32.
		PublicationPendingBytes: uint64(pub.PendingBytes), // #nosec G115 -- a byte count is not negative.
		WriteState:              volume.VolumeWriteState_VOLUME_WRITE_STATE_WRITABLE,
	}
	if !st.sizesAt.IsZero() {
		h.SizesKnown = true
		h.SizesAt = timestamppb.New(st.sizesAt)
		h.FileBytes, h.LiveValueBytes, h.FreeDiskBytes = st.fileBytes, st.liveValueBytes, st.freeDiskBytes
	}
	return h, waitCh
}

// enterGCPhase moves the volume to phase and returns the function that moves
// it back to the phase it left, for the caller to defer.
func (v *Volume) enterGCPhase(phase volume.VolumeGCPhase) func() {
	prev := v.setGCPhase(phase)
	return func() { v.setGCPhase(prev) }
}

// setGCPhase moves the volume to phase, restarting the time in phase when the
// phase changes, and returns the phase it left.
func (v *Volume) setGCPhase(phase volume.VolumeGCPhase) volume.VolumeGCPhase {
	var prev volume.VolumeGCPhase
	v.healthBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		prev = v.health.phase
		if prev != phase {
			v.health.phase, v.health.phaseStart = phase, time.Now()
			broadcast()
		}
	})
	return prev
}

// wakeHealthSizes asks the size measurement to run soon, without waiting.
func (v *Volume) wakeHealthSizes() {
	if v.healthWake == nil {
		return
	}
	select {
	case v.healthWake <- struct{}{}:
	default:
	}
}

// runHealthSizes measures the storage when a measurement is due, until ctx
// ends. A failed measurement keeps the last sizes and is retried on the next
// poll.
func (v *Volume) runHealthSizes(ctx context.Context) {
	// Poll from an initial measurement until ctx ends.
	defer close(v.healthDone)
	ticker := time.NewTicker(healthPoll)
	defer ticker.Stop()
	var lastBytes uint64
	due := true
	for {
		// Measure when due, counting writes from the measurement.
		committed := v.GetPublicationStats().CommittedBytes
		if due || v.healthWriteDue(committed-lastBytes) {
			if v.measureHealthSizes(ctx) {
				lastBytes = committed
			}
		}

		// Wait for the next poll or a wake.
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			due = false
		case <-v.healthWake:
			due = true
		}
	}
}

// healthWriteDue reports whether the volume wrote enough since the last
// measurement, or the sizes are old enough, to measure again.
func (v *Volume) healthWriteDue(written uint64) bool {
	// Copy the measured sizes.
	var st healthState
	v.healthBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		st = v.health
	})

	// Measure sizes never measured or old, or after enough writes.
	if st.sizesAt.IsZero() || time.Since(st.sizesAt) >= healthMaxAge {
		return true
	}
	return written >= max(st.freeDiskBytes/healthHeadroomDivisor, healthMinWrite)
}

// measureHealthSizes measures the storage and publishes the sizes, reporting
// whether the measurement succeeded.
func (v *Volume) measureHealthSizes(ctx context.Context) bool {
	// Measure, keeping the last sizes on a failure.
	fileBytes, liveValueBytes, freeDiskBytes, err := v.healthSizes(ctx)
	if err != nil {
		return false
	}

	// Publish the sizes.
	v.healthBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		v.health.sizesAt = time.Now()
		v.health.fileBytes, v.health.liveValueBytes, v.health.freeDiskBytes = fileBytes, liveValueBytes, freeDiskBytes
		broadcast()
	})
	return true
}

// _ is a type assertion
var _ volume.HealthWatcher = (*Volume)(nil)
