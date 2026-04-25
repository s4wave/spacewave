package resource_root

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	resource_cdn "github.com/s4wave/spacewave/core/resource/cdn"
	resource_debugdb "github.com/s4wave/spacewave/core/resource/debugdb"
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
	// stateAtomMgr manages state atom stores
	stateAtomMgr *resource_state.StateAtomManager
	// stateAtomStoreIndexMtx guards stateAtomStoreIndex setup.
	stateAtomStoreIndexMtx sync.Mutex
	// stateAtomStoreIndex tracks known root state atom store ids.
	stateAtomStoreIndex *session.StateAtomStoreIndex
	// releaseStateAtomStoreIndex releases the root object store handle.
	releaseStateAtomStoreIndex func()
	// cdnRegistry owns the process-scoped map of CdnInstances.
	cdnRegistry *resource_cdn.Registry
	// webListeners owns daemon-background localhost web listeners.
	webListeners *webListenerRegistry
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
	return s
}

// Close releases process-owned root resources.
func (s *CoreRootServer) Close() {
	if s.webListeners != nil {
		s.webListeners.close()
	}
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
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	debugResource := resource_debugdb.NewDebugDbResource(s.le, s.b)
	id, err := resourceCtx.AddResource(debugResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_root.GetDebugDbResponse{ResourceId: id}, nil
}

// _ is a type assertion
var _ s4wave_root.SRPCRootResourceServiceServer = ((*CoreRootServer)(nil))
