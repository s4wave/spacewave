package resource_testbed

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

var nextTestbedEngineId atomic.Int64

// StateAtomObjectStoreID is the object store ID for testbed state atoms.
const StateAtomObjectStoreID = "testbed-state-atoms"

// TestbedResourceServer implements the TestbedResourceService.
// It acts as the root resource for creating world engine resources.
type TestbedResourceServer struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// volumeID is the volume ID for engine storage
	volumeID string
	// bucketID is the bucket ID for engine storage
	bucketID string
	// ctx is the context for long-lived resources
	ctx context.Context
	// testResult handles test result broadcasting
	testResult broadcast.Broadcast
	// testSuccess stores whether the test passed
	testSuccess bool
	// testError stores the test error message
	testError string
	// testCompleted is true when MarkTestResult has been called
	testCompleted bool
	// stateAtomMgr manages state atom stores
	stateAtomMgr *resource_state.StateAtomManager
}

// NewTestbedResourceServer creates a new TestbedResourceServer.
// ctx is used for long-lived resources like BusEngine instances.
func NewTestbedResourceServer(ctx context.Context, le *logrus.Entry, bus bus.Bus, volumeID string, bucketID string) *TestbedResourceServer {
	return &TestbedResourceServer{
		le:           le,
		bus:          bus,
		volumeID:     volumeID,
		bucketID:     bucketID,
		ctx:          ctx,
		stateAtomMgr: resource_state.NewStateAtomManager(bus, StateAtomObjectStoreID, volumeID),
	}
}

// CreateWorld creates a new world engine and returns an EngineResource.
func (s *TestbedResourceServer) CreateWorld(ctx context.Context, req *s4wave_testbed.CreateWorldRequest) (*s4wave_testbed.CreateWorldResponse, error) {
	// Generate engine ID if not provided
	engineID := req.EngineId
	if engineID == "" {
		nextID := nextTestbedEngineId.Add(1)
		engineID = fmt.Sprintf("%s-engine-%d", s.bucketID, nextID)
	}

	// Setup world engine configuration
	volumeID := s.volumeID
	bucketID := s.bucketID
	objectStoreID := engineID + "-store"

	// Create bucket if it doesn't exist
	bucketConf, err := bucket.NewConfig(bucketID, 1, nil, nil)
	if err != nil {
		return nil, err
	}
	_, err = bucket.ExApplyBucketConfig(ctx, s.bus, bucket.NewApplyBucketConfigToVolume(bucketConf, volumeID))
	if err != nil {
		return nil, err
	}

	// Create world engine config
	engConf := world_block_engine.NewConfig(
		engineID,
		volumeID,
		bucketID,
		objectStoreID,
		&bucket.ObjectRef{BucketId: bucketID},
		nil,
		false,
	)

	// Start the world engine controller using server's context (not request context)
	// This ensures the engine remains alive even if the request context is canceled
	_, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(s.ctx, s.bus, engConf)
	if err != nil {
		return nil, err
	}

	// Create bus-based engine using server's context
	busEngine := world.NewBusEngine(s.ctx, s.bus, engineID)

	// Create engine resource
	engineInfo := &s4wave_world.EngineInfo{
		EngineId: engineID,
		BucketId: bucketID,
	}
	engineResource := resource_world.NewEngineResource(s.le, s.bus, busEngine, space_world_optypes.LookupWorldOp, engineInfo)

	// Release function for cleanup
	releaseFunc := func() {
		worldCtrlRef.Release()
	}

	// Add resource via the resource system.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		worldCtrlRef.Release()
		return nil, err
	}

	id, err := resourceCtx.AddResource(engineResource.GetMux(), releaseFunc)
	if err != nil {
		worldCtrlRef.Release()
		return nil, err
	}

	return &s4wave_testbed.CreateWorldResponse{ResourceId: id}, nil
}

// MarkTestResult marks the test result (success or failure).
func (s *TestbedResourceServer) MarkTestResult(ctx context.Context, req *s4wave_testbed.MarkTestResultRequest) (*s4wave_testbed.MarkTestResultResponse, error) {
	s.testResult.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.testCompleted = true
		s.testSuccess = req.Success
		s.testError = req.ErrorMessage

		if req.Success {
			s.le.Info("test marked as successful")
		} else {
			s.le.Errorf("test marked as failed: %s", req.ErrorMessage)
		}

		// Signal any waiters
		broadcast()
	})

	return &s4wave_testbed.MarkTestResultResponse{}, nil
}

// WaitForTestResult waits for the test to complete and returns the result.
// This is useful for the Go test harness to wait for the TypeScript test to finish.
func (s *TestbedResourceServer) WaitForTestResult(ctx context.Context) (success bool, errorMsg string, err error) {
	err = s.testResult.Wait(ctx, func(broadcast func(), getWaitCh func() <-chan struct{}) (bool, error) {
		if s.testCompleted {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return false, "", err
	}

	// Read the result while holding the lock
	s.testResult.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		success = s.testSuccess
		errorMsg = s.testError
	})

	return success, errorMsg, nil
}

// Register registers the server with the mux.
func (s *TestbedResourceServer) Register(mux srpc.Mux) error {
	return s4wave_testbed.SRPCRegisterTestbedResourceService(mux, s)
}

// GetMux returns the mux for this root resource.
func (s *TestbedResourceServer) GetMux() srpc.Invoker {
	mux := srpc.NewMux()
	_ = s.Register(mux)
	return mux
}

// _ is a type assertion
var _ s4wave_testbed.SRPCTestbedResourceServiceServer = ((*TestbedResourceServer)(nil))

// AccessStateAtom accesses a state atom resource.
func (s *TestbedResourceServer) AccessStateAtom(
	ctx context.Context,
	req *s4wave_testbed.AccessStateAtomRequest,
) (*s4wave_testbed.AccessStateAtomResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	storeID := req.GetStoreId()
	if storeID == "" {
		storeID = resource_state.DefaultStateAtomStoreID
	}

	store, err := s.stateAtomMgr.GetOrCreateStore(ctx, storeID)
	if err != nil {
		return nil, err
	}

	stateResource := resource_state.NewStateAtomResource(store)
	id, err := resourceCtx.AddResource(stateResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_testbed.AccessStateAtomResponse{ResourceId: id}, nil
}
