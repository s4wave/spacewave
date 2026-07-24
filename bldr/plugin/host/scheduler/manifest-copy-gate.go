package plugin_host_scheduler

import "context"

// ManifestCopyGate delays startup manifest copies until its readiness boundary.
type ManifestCopyGate interface {
	// IsReady reports whether new copies can start without waiting.
	IsReady() bool
	// WaitReady waits until copies can start or the context is canceled.
	WaitReady(ctx context.Context) error
}
