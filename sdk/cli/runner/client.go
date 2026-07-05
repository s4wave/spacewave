package runner

import (
	"context"

	"github.com/aperturerobotics/cli"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

// ClientFactory opens a command-scoped Spacewave SDK client for the selected transport.
type ClientFactory interface {
	NewClient(ctx context.Context, c *cli.Context) (Client, error)
	StatusEndpoint(ctx context.Context, c *cli.Context) (string, error)
}

// Client is the minimal Spacewave session surface needed by shared CLI commands.
type Client interface {
	Close()
	MountSession(ctx context.Context, idx uint32) (Session, error)
}

// Session is the minimal mounted session surface needed by shared CLI commands.
type Session interface {
	Release()
	GetSessionInfo(ctx context.Context) (*s4wave_session.GetSessionInfoResponse, error)
	WatchResourcesList(ctx context.Context) (ResourcesListStream, error)
	WatchLockState(ctx context.Context) (LockStateStream, error)
	WatchRecoveryStatus(ctx context.Context) (*s4wave_status.RecoveryStatus, error)
}

// ResourcesListStream streams session resource-list snapshots.
type ResourcesListStream interface {
	Close() error
	Recv() (*s4wave_session.WatchResourcesListResponse, error)
}

// LockStateStream streams session lock-state snapshots.
type LockStateStream interface {
	Close() error
	Recv() (*s4wave_session.WatchLockStateResponse, error)
}
