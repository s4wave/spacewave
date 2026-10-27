package volume_controller

import "github.com/s4wave/spacewave/db/volume"

// reserveWatch tells the GC loop when its volume enters the disk reserve, so
// a volume that refuses writes collects at once instead of at the next
// scheduled block pass.
type reserveWatch struct {
	// hw is the volume's health, nil when it publishes none.
	hw volume.HealthWatcher
	// in records the write state of the last snapshot.
	in bool
}

// entered snapshots the health and reports whether the volume entered the
// reserve since the last call, with a channel that closes when the health
// next changes. A volume without health never enters and returns a nil
// channel, which a select never receives from.
func (w *reserveWatch) entered() (bool, <-chan struct{}) {
	// Snapshot the write state.
	if w.hw == nil {
		return false, nil
	}
	h, waitCh := w.hw.GetHealthSnapshotWithWait()

	// Report a move into the reserve once.
	in := h.GetWriteState() == volume.VolumeWriteState_VOLUME_WRITE_STATE_RESERVE
	entered := in && !w.in
	w.in = in
	return entered, waitCh
}
