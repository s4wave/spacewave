package kvtx

import (
	"context"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
)

// HealthSizesFn measures a volume's storage and chooses its disk reserve.
type HealthSizesFn func(ctx context.Context) (HealthSizes, error)

// healthPoll is how often the size measurement retries a failed measurement
// and checks the age of the last.
const healthPoll = time.Second

// healthMaxAge is the age past which measured sizes are measured again,
// however little the volume wrote, since other processes use the disk too.
const healthMaxAge = 30 * time.Second

// healthHeadroomDivisor sets how much of the free disk the volume's own writes
// may use before the sizes are measured again: one part in this many.
const healthHeadroomDivisor = 8

// healthMinWrite is the least the volume writes between measurements, so a
// nearly full disk is not measured on every write.
const healthMinWrite = 1 << 20

// reserveMarginDivisor sets the free disk above the reserve that a volume in
// the reserve state needs to admit writes again: one part in this many of the
// reserve, so a volume at the edge does not flap.
const reserveMarginDivisor = 4

// healthState is the collection phase and the measured sizes of a volume,
// guarded by the volume's healthBcast.
type healthState struct {
	// phase is the collection step the volume is in.
	phase volume.VolumeGCPhase
	// phaseStart is when the volume entered phase.
	phaseStart time.Time
	// sizesAt is when the sizes were measured, zero before the first.
	sizesAt time.Time
	// sizes is the last measurement.
	sizes HealthSizes
	// reserve records that free disk fell below the reserve and has not
	// passed the reserve and its margin since.
	reserve bool
}

// SetHealthSizes starts measuring the storage with fn, which the health
// snapshot reports. It measures once at once, then when the volume's written
// bytes pass a fraction of the free disk above the reserve, when a
// collection may have freed space, and at least every 30 seconds. Each
// measurement moves the write state, which the new-data entry points read. It
// is called once, before the volume is shared, and Close stops it.
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
	if st.reserve {
		h.WriteState = volume.VolumeWriteState_VOLUME_WRITE_STATE_RESERVE
	}
	if !st.sizesAt.IsZero() {
		h.SizesKnown = true
		h.SizesAt = timestamppb.New(st.sizesAt)
		h.FileBytes, h.LiveValueBytes = st.sizes.FileBytes, st.sizes.LiveValueBytes
		h.FreeDiskBytes, h.ReserveBytes = st.sizes.FreeDiskBytes, st.sizes.ReserveBytes
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
// ends: at once, on a wake, when the sizes are old, and on each poll in the
// reserve state. A failed measurement keeps the last sizes and is retried on
// the next poll.
func (v *Volume) runHealthSizes(ctx context.Context) {
	// Poll from an initial measurement until ctx ends.
	defer close(v.healthDone)
	ticker := time.NewTicker(healthPoll)
	defer ticker.Stop()
	var measuredAt time.Time
	due := true
	for {
		// Measure when due or in the reserve state, counting the writes from
		// before the measurement toward the next.
		if due || v.reserved.Load() || time.Since(measuredAt) >= healthMaxAge {
			written := v.writtenBytes.Load()
			if sizes, ok := v.measureHealthSizes(ctx); ok {
				measuredAt = time.Now()
				v.healthDueAt.Store(written + healthDueBytes(sizes))
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

// healthDueBytes returns how much the volume may write after a measurement of
// sizes before it measures again. Writes count against the free disk above
// the reserve, so a volume nearing the reserve measures more often.
func healthDueBytes(sizes HealthSizes) uint64 {
	headroom := sizes.FreeDiskBytes - min(sizes.ReserveBytes, sizes.FreeDiskBytes)
	return max(headroom/healthHeadroomDivisor, healthMinWrite)
}

// noteWritten counts n bytes of new data the volume admitted or committed,
// and wakes the size measurement once the count passes the mark the last
// measurement set, so a fast writer is measured before it passes the reserve.
func (v *Volume) noteWritten(n uint64) {
	if v.writtenBytes.Add(n) >= v.healthDueAt.Load() {
		v.wakeHealthSizes()
	}
}

// measureHealthSizes measures the storage, moves the write state and
// publishes both, returning the sizes and whether the measurement succeeded.
func (v *Volume) measureHealthSizes(ctx context.Context) (HealthSizes, bool) {
	// Measure, keeping the last sizes on a failure.
	sizes, err := v.healthSizes(ctx)
	if err != nil {
		return HealthSizes{}, false
	}

	// Publish the sizes and the write state they select, setting the gate
	// under the same lock so the snapshot and the gate agree.
	v.healthBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		v.health.sizesAt, v.health.sizes = time.Now(), sizes
		v.health.reserve = inReserve(sizes, v.health.reserve)
		v.reserved.Store(v.health.reserve)
		broadcast()
	})
	return sizes, true
}

// inReserve reports whether a volume measuring sizes is in the reserve state,
// given whether it was. It enters below the reserve and leaves above the
// reserve and its margin.
func inReserve(sizes HealthSizes, was bool) bool {
	limit := sizes.ReserveBytes
	if was {
		limit += sizes.ReserveBytes / reserveMarginDivisor
	}
	return sizes.ReserveBytes != 0 && sizes.FreeDiskBytes < limit
}

// checkReserve refuses a new write of size bytes with volume.ErrReserve while
// the volume is in the reserve state, unless volume.WithReserveExempt marked
// ctx. It counts the bytes it admits toward the next size measurement; a
// publication passes zero, since the publication writer counts its commits.
func (v *Volume) checkReserve(ctx context.Context, size int) error {
	if v.reserved.Load() && !volume.ReserveExempt(ctx) {
		return volume.ErrReserve
	}
	if size != 0 {
		v.noteWritten(uint64(size)) // #nosec G115 -- a length is not negative.
	}
	return nil
}

// batchBytes returns the length of the data entries write.
func batchBytes(entries []*block.PutBatchEntry) int {
	var n int
	for _, e := range entries {
		n += len(e.Data)
	}
	return n
}

// _ is a type assertion
var _ volume.HealthWatcher = (*Volume)(nil)
