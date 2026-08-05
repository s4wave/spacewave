package resource_root

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	resource_cdn "github.com/s4wave/spacewave/core/resource/cdn"
	resource_debugdb "github.com/s4wave/spacewave/core/resource/debugdb"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"github.com/sirupsen/logrus"
)

// CoreRootServer implements the RootResourceService for s4wave core.
type CoreRootServer struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus to look up and perform actions on
	b bus.Bus
	// hostPluginID is the plugin id that owns this resource root.
	hostPluginID string
	// stateAtomMgr manages state atom stores
	stateAtomMgr *resource_state.StateAtomManager
	// spaceRootAliasBcast broadcasts configured root registry changes.
	spaceRootAliasBcast broadcast.Broadcast
	// stateAtomStoreIndexMtx guards stateAtomStoreIndex setup.
	stateAtomStoreIndexMtx sync.Mutex
	// stateAtomStoreIndex tracks known root state atom store ids.
	stateAtomStoreIndex *session.StateAtomStoreIndex
	// releaseStateAtomStoreIndex releases the root object store handle.
	releaseStateAtomStoreIndex func()
	// stateAtomStoreClosed rejects lazy store acquisition after shutdown.
	stateAtomStoreClosed bool
	// stateAtomStoreIndexBuilder overrides the external acquisition in
	// CoreRootServer tests.
	stateAtomStoreIndexBuilder func(context.Context) (*session.StateAtomStoreIndex, func(), error)
	// cdnRegistry owns the process-scoped map of CdnInstances.
	cdnRegistry *resource_cdn.Registry
	// webListeners owns daemon-background localhost web listeners.
	webListeners *webListenerRegistry
	// recoveryStatusRegistry owns volatile renderer recovery facts by logical
	// session across separately mounted SessionResources.
	recoveryStatusRegistry *resource_session.RecoveryStatusRegistry
}

// NewCoreRootServer creates a new CoreRootServer.
func NewCoreRootServer(le *logrus.Entry, b bus.Bus) *CoreRootServer {
	s := &CoreRootServer{
		le: le,
		b:  b,
	}
	s.stateAtomMgr = newStateAtomManager(s)
	s.cdnRegistry = resource_cdn.NewRegistry(le, b)
	s.webListeners = newWebListenerRegistry(le)
	s.recoveryStatusRegistry = resource_session.NewRecoveryStatusRegistry()
	return s
}

// SetHostPluginID records the plugin id serving this resource root.
func (s *CoreRootServer) SetHostPluginID(pluginID string) {
	s.hostPluginID = pluginID
}

// Close releases process-owned root resources.
func (s *CoreRootServer) Close() {
	if s.webListeners != nil {
		s.webListeners.close()
	}
	if s.cdnRegistry != nil {
		s.cdnRegistry.Close()
	}
	s.closeStateAtomStoreIndex()
}

// Register registers the server with the mux.
func (s *CoreRootServer) Register(mux srpc.Mux) error {
	return s4wave_root.SRPCRegisterRootResourceService(mux, s)
}

// GetDebugDb returns a debug database resource for storage diagnostics.
func (s *CoreRootServer) GetDebugDb(
	ctx context.Context,
	_ *s4wave_root.GetDebugDbRequest,
) (*s4wave_root.GetDebugDbResponse, error) {
	// Acquire the caller resource context.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Construct and register the debug database resource.
	debugResource := resource_debugdb.NewDebugDbResource(s.le, s.b)
	id, err := resourceCtx.AddResource(debugResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	// Return the registered resource identifier.
	return &s4wave_root.GetDebugDbResponse{ResourceId: id}, nil
}

// _ is a type assertion
var _ s4wave_root.SRPCRootResourceServiceServer = ((*CoreRootServer)(nil))
