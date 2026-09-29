package spacewave_launcher_gossip

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// launcher is the local launcher as seen by an exchange.
type launcher interface {
	// Snapshot returns the current launcher info, nil when no launcher is
	// reachable, and a channel closed on the next change.
	Snapshot() (*spacewave_launcher.LauncherInfo, <-chan struct{})
	// Push offers a signed DistConfig message to the launcher.
	// Returns true if the launcher adopted it.
	Push(ctx context.Context, msg string) (bool, error)
}

// busLauncher is the launcher reached through its RPC service on a bus.
type busLauncher struct {
	// InfoWatcher mirrors the launcher's LauncherInfo.
	*spacewave_launcher.InfoWatcher
	// b is the bus used to push offered configs.
	b bus.Bus
}

// Push offers a signed DistConfig message to the launcher.
func (l *busLauncher) Push(ctx context.Context, msg string) (bool, error) {
	resp, err := spacewave_launcher.ExPushDistConfigMsg(ctx, l.b, msg)
	return resp.GetUpdated(), err
}

// _ is a type assertion
var _ launcher = (*busLauncher)(nil)
