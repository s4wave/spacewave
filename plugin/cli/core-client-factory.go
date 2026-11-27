package cli_plugin

import (
	"context"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/sdk/cli/runner"
	s4wave_plugin "github.com/s4wave/spacewave/sdk/plugin"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

const corePluginID = "spacewave-core"

// CoreClientFactory opens runner clients against the Spacewave core plugin resource service.
type CoreClientFactory struct {
	b bus.Bus
}

// NewCoreClientFactory constructs a runner client factory for the Spacewave core plugin.
func NewCoreClientFactory(b bus.Bus) *CoreClientFactory {
	return &CoreClientFactory{b: b}
}

// NewClient opens a command-scoped Spacewave SDK client for the browser plugin runtime.
func (f *CoreClientFactory) NewClient(ctx context.Context, c *cli.Context) (runner.Client, error) {
	// Connect the command client to the core plugin resource service.
	resources, err := s4wave_plugin.ConnectPluginResources(ctx, f.b, corePluginID)
	if err != nil {
		return nil, err
	}

	// Open the root resource and release the connection if initialization fails.
	rootRef := resources.AccessRootResource()
	root, err := s4wave_root.NewRoot(resources, rootRef)
	if err != nil {
		rootRef.Release()
		resources.Release()
		return nil, errors.Wrap(err, "root resource")
	}

	return &coreClient{resources: resources, root: root}, nil
}

// StatusEndpoint returns the browser runtime endpoint description used by status output.
func (f *CoreClientFactory) StatusEndpoint(ctx context.Context, c *cli.Context) (string, error) {
	return corePluginID, nil
}

type coreClient struct {
	resources *s4wave_plugin.PluginResources
	root      *s4wave_root.Root
}

// Close releases the core plugin resource connection.
func (c *coreClient) Close() {
	if c.root != nil {
		c.root.Release()
	}
	if c.resources != nil {
		c.resources.Release()
	}
}

// MountSession retains the selected Session for a runner command.
func (c *coreClient) MountSession(ctx context.Context, idx uint32) (runner.Session, error) {
	// Mount the requested session and reject a missing session index.
	resp, err := c.root.MountSessionByIdx(ctx, idx)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	if resp.GetNotFound() {
		return nil, errors.Errorf("no session found at index %d", idx)
	}

	// Adopt the mounted session resource into a runner session.
	sessRef := c.resources.CreateResourceReference(resp.GetResourceId())
	sess, err := s4wave_session.NewSession(c.resources, sessRef)
	if err != nil {
		sessRef.Release()
		return nil, errors.Wrap(err, "session resource")
	}
	return &coreSession{Session: sess}, nil
}

type coreSession struct {
	*s4wave_session.Session
}

// WatchResourcesList opens the Session resource listing for the runner.
func (s *coreSession) WatchResourcesList(ctx context.Context) (runner.ResourcesListStream, error) {
	return s.Session.WatchResourcesList(ctx)
}

// WatchLockState opens the Session lock-state watch for the runner.
func (s *coreSession) WatchLockState(ctx context.Context) (runner.LockStateStream, error) {
	return s.Session.WatchLockState(ctx)
}

// WatchRecoveryStatus reads the first recovery update and closes its stream.
func (s *coreSession) WatchRecoveryStatus(ctx context.Context) (*s4wave_status.RecoveryStatus, error) {
	// Open the mounted Session's recovery status watch.
	strm, err := s.Session.WatchRecoveryStatus(ctx)
	if err != nil {
		return nil, err
	}
	defer strm.Close()

	// Read the initial recovery status before releasing the watch.
	resp, err := strm.Recv()
	if err != nil {
		return nil, err
	}
	return resp.GetStatus(), nil
}

// _ is a type assertion.
var _ runner.ClientFactory = (*CoreClientFactory)(nil)
