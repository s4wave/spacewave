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
	resources, err := s4wave_plugin.ConnectPluginResources(ctx, f.b, corePluginID)
	if err != nil {
		return nil, err
	}

	rootRef := resources.Client.AccessRootResource()
	root, err := s4wave_root.NewRoot(resources.Client, rootRef)
	if err != nil {
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

func (c *coreClient) Close() {
	if c.root != nil {
		c.root.Release()
	}
	if c.resources != nil {
		c.resources.Release()
	}
}

func (c *coreClient) MountSession(ctx context.Context, idx uint32) (runner.Session, error) {
	resp, err := c.root.MountSessionByIdx(ctx, idx)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	if resp.GetNotFound() {
		return nil, errors.Errorf("no session found at index %d", idx)
	}

	sessRef := c.resources.Client.CreateResourceReference(resp.GetResourceId())
	sess, err := s4wave_session.NewSession(c.resources.Client, sessRef)
	if err != nil {
		sessRef.Release()
		return nil, errors.Wrap(err, "session resource")
	}
	return &coreSession{Session: sess}, nil
}

type coreSession struct {
	*s4wave_session.Session
}

func (s *coreSession) WatchResourcesList(ctx context.Context) (runner.ResourcesListStream, error) {
	return s.Session.WatchResourcesList(ctx)
}

func (s *coreSession) WatchLockState(ctx context.Context) (runner.LockStateStream, error) {
	return s.Session.WatchLockState(ctx)
}

func (s *coreSession) WatchRecoveryStatus(ctx context.Context) (*s4wave_status.RecoveryStatus, error) {
	client, err := s.GetResourceRef().GetClient()
	if err != nil {
		return nil, err
	}
	strm, err := s4wave_status.NewSRPCSystemStatusServiceClient(client).WatchRecoveryStatus(
		ctx,
		&s4wave_status.WatchRecoveryStatusRequest{},
	)
	if err != nil {
		return nil, err
	}
	defer strm.Close()
	resp, err := strm.Recv()
	if err != nil {
		return nil, err
	}
	return resp.GetStatus(), nil
}

// _ is a type assertion.
var _ runner.ClientFactory = (*CoreClientFactory)(nil)
