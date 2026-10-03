package inmem

import "sync"

var (
	volumeCoordinatorsMu sync.Mutex
	volumeCoordinators   = map[string]*Coordinator{}
)

// ForVolume returns the process-local coordinator for a Volume id.
func ForVolume(volumeID string) *Coordinator {
	// Give an unnamed Volume its own independent coordinator.
	if volumeID == "" {
		return NewCoordinator()
	}

	// Reuse or register the process-local coordinator for this Volume.
	volumeCoordinatorsMu.Lock()
	defer volumeCoordinatorsMu.Unlock()
	coordinator := volumeCoordinators[volumeID]
	if coordinator == nil {
		coordinator = NewCoordinator()
		volumeCoordinators[volumeID] = coordinator
	}
	return coordinator
}
