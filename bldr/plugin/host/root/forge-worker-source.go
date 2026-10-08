package plugin_host_root

import "context"

// ForgeWorkerSource exposes the daemon-owned Forge Worker declaration without
// mutation authority.
type ForgeWorkerSource interface {
	// WaitForgeWorker returns the binary s4wave.device.ForgeWorkerDeclaration,
	// the Device object key and the revision once the revision differs from
	// last. A zero last returns the current value at once. The declaration is
	// empty while the Device declares no Worker.
	WaitForgeWorker(ctx context.Context, last uint64) (declaration []byte, deviceObjectKey string, revision uint64, err error)
}
