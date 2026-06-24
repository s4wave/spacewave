package inmem

import "sync"

var (
	volumeCoordinatorsMu sync.Mutex
	volumeCoordinators   = map[string]*Coordinator{}
)

// ForVolume returns the process-local coordinator for a Volume id.
func ForVolume(volumeID string) *Coordinator {
	if volumeID == "" {
		return NewCoordinator()
	}

	volumeCoordinatorsMu.Lock()
	defer volumeCoordinatorsMu.Unlock()
	coordinator := volumeCoordinators[volumeID]
	if coordinator == nil {
		coordinator = NewCoordinator()
		volumeCoordinators[volumeID] = coordinator
	}
	return coordinator
}
