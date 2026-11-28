//go:build !js && !wasip1

package volume_s4db

// SetBoltStep replaces the function convertBolt calls after each step and
// returns the function that restores the previous one.
func SetBoltStep(step func(string)) func() {
	prev := boltStep
	boltStep = step
	return func() { boltStep = prev }
}

// SetSyncDir replaces the function that syncs a directory and returns the
// function that restores the previous one.
func SetSyncDir(sync func(string) error) func() {
	prev := syncDir
	syncDir = sync
	return func() { syncDir = prev }
}
