package inmem

import "sync"

var volumeCoordinators sync.Map

// ForVolume returns the process-local coordinator for a Volume id.
func ForVolume(volumeID string) *Coordinator {
	if volumeID == "" {
		return NewCoordinator()
	}

	actual, _ := volumeCoordinators.LoadOrStore(volumeID, NewCoordinator())
	return actual.(*Coordinator)
}
