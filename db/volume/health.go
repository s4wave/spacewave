package volume

import (
	"context"
	"slices"
	"strings"
	"sync"
)

// HealthWatcher is implemented by a volume that publishes its VolumeHealth.
type HealthWatcher interface {
	// GetHealthSnapshotWithWait returns the volume's health and a channel that
	// closes when its collection phase or measured sizes change.
	GetHealthSnapshotWithWait() (*VolumeHealth, <-chan struct{})
}

// WaitWritable waits until vol leaves its disk reserve. It returns at once for
// a volume that is writable or publishes no health, and returns ctx's error if
// ctx ends first. A writer that failed with ErrReserve waits here instead of
// retrying writes the volume will refuse.
func WaitWritable(ctx context.Context, vol any) error {
	hw, ok := vol.(HealthWatcher)
	if !ok {
		return nil
	}
	for {
		h, waitCh := hw.GetHealthSnapshotWithWait()
		if h.GetWriteState() != VolumeWriteState_VOLUME_WRITE_STATE_RESERVE {
			return nil
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// healthSources holds the volumes a process collects, for consumers that
// report every volume's health, such as the process metrics.
var healthSources struct {
	// mtx guards set.
	mtx sync.Mutex
	// set holds each registered watcher with its registration count.
	set map[HealthWatcher]int
}

// RegisterHealth adds w to the volumes whose health HealthSnapshots reports,
// and whose flows FlowSnapshots reports when w is a FlowReporter, until the
// returned function is called.
func RegisterHealth(w HealthWatcher) func() {
	// Count the registration.
	healthSources.mtx.Lock()
	if healthSources.set == nil {
		healthSources.set = make(map[HealthWatcher]int)
	}
	healthSources.set[w]++
	healthSources.mtx.Unlock()

	// Release it once.
	var once sync.Once
	return func() {
		once.Do(func() {
			healthSources.mtx.Lock()
			if healthSources.set[w]--; healthSources.set[w] <= 0 {
				delete(healthSources.set, w)
			}
			healthSources.mtx.Unlock()
		})
	}
}

// HealthSnapshots returns the health of every registered volume, sorted by
// volume ID.
func HealthSnapshots() []*VolumeHealth {
	// Copy the registered watchers, so no snapshot runs under the lock.
	healthSources.mtx.Lock()
	watchers := make([]HealthWatcher, 0, len(healthSources.set))
	for w := range healthSources.set {
		watchers = append(watchers, w)
	}
	healthSources.mtx.Unlock()

	// Snapshot each and sort them.
	out := make([]*VolumeHealth, 0, len(watchers))
	for _, w := range watchers {
		h, _ := w.GetHealthSnapshotWithWait()
		out = append(out, h)
	}
	slices.SortFunc(out, func(a, b *VolumeHealth) int {
		return strings.Compare(a.GetVolumeId(), b.GetVolumeId())
	})
	return out
}
