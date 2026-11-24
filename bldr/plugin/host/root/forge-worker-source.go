package plugin_host_root

import "context"

// ForgeWorkerSource exposes the daemon-owned Forge Worker declaration of each
// Space World without mutation authority.
type ForgeWorkerSource interface {
	// WaitForgeWorker returns the binary s4wave.device.ForgeWorkerDeclaration,
	// the Device object key and the revision of the World engineID once the
	// revision differs from last. A zero last returns the current value at
	// once. The declaration is empty while the Device declares no Worker there.
	WaitForgeWorker(ctx context.Context, engineID string, last uint64) (declaration []byte, deviceObjectKey string, revision uint64, err error)
}
