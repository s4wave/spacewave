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

var nextTestbedEngineID atomic.Int64

// StateAtomObjectStoreID is the object store ID for testbed state atoms.
const StateAtomObjectStoreID = "testbed-state-atoms"

type queuedTestResult struct {
	success  bool
	errorMsg string
}

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
	// testResult broadcasts legacy and queued test result state.
	testResult broadcast.Broadcast
	// testSuccess stores whether the legacy test passed.
	testSuccess bool
	// testError stores the legacy test error message.
	testError string
	// testCompleted is true when MarkTestResult has been called without a test name.
	testCompleted bool
	// queuedTestNames contains test names waiting for a plugin worker.
	queuedTestNames []string
	// queuedTestResults stores completed named test results.
	queuedTestResults map[string]queuedTestResult
	// queuedTestsClosed is true when no more named tests will be queued.
	queuedTestsClosed bool
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
		nextID := nextTestbedEngineID.Add(1)
		engineID = fmt.Sprintf("%s-engine-%d", s.bucketID, nextID)
	}

	// Setup world engine configuration
	volumeID := s.volumeID
	bucketID := s.bucketID
	objectStoreID := engineID + "-store"

	// Create bucket if it doesn't exist
	bucketConf, err := bucket.NewConfig(bucketID, 1, nil)
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

// MarkTestResult marks a legacy or named queued test result.
func (s *TestbedResourceServer) MarkTestResult(ctx context.Context, req *s4wave_testbed.MarkTestResultRequest) (*s4wave_testbed.MarkTestResultResponse, error) {
	if req.TestName != "" {
		s.markQueuedTestResult(req.TestName, req.Success, req.ErrorMessage)
		return &s4wave_testbed.MarkTestResultResponse{}, nil
	}

	s.FailQueuedTests(req.Success, req.ErrorMessage)

	return &s4wave_testbed.MarkTestResultResponse{}, nil
}

// ClaimTest waits for the next queued test name.
func (s *TestbedResourceServer) ClaimTest(ctx context.Context, req *s4wave_testbed.ClaimTestRequest) (*s4wave_testbed.ClaimTestResponse, error) {
	for {
		err := s.testResult.Wait(ctx, func(broadcast func(), getWaitCh func() <-chan struct{}) (bool, error) {
			return len(s.queuedTestNames) != 0 || s.queuedTestsClosed, nil
		})
		if err != nil {
			return nil, err
		}

		var testName string
		var closed bool
		s.testResult.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if len(s.queuedTestNames) != 0 {
				testName = s.queuedTestNames[0]
				s.queuedTestNames = s.queuedTestNames[1:]
				return
			}
			closed = s.queuedTestsClosed
		})
		if testName != "" || closed {
			return &s4wave_testbed.ClaimTestResponse{
				TestName: testName,
				Closed:   closed,
			}, nil
		}
	}
}

// RunTest queues a named test and waits for its result.
func (s *TestbedResourceServer) RunTest(ctx context.Context, testName string) (success bool, errorMsg string, err error) {
	s.testResult.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if s.queuedTestResults == nil {
			s.queuedTestResults = make(map[string]queuedTestResult)
		}
		if _, ok := s.queuedTestResults[testName]; !ok {
			s.queuedTestNames = append(s.queuedTestNames, testName)
			broadcast()
		}
	})

	err = s.testResult.Wait(ctx, func(broadcast func(), getWaitCh func() <-chan struct{}) (bool, error) {
		if s.testCompleted && !s.testSuccess {
			return true, nil
		}
		_, ok := s.queuedTestResults[testName]
		return ok, nil
	})
	if err != nil {
		return false, "", err
	}

	s.testResult.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if result, ok := s.queuedTestResults[testName]; ok {
			success = result.success
			errorMsg = result.errorMsg
			return
		}
		success = s.testSuccess
		errorMsg = s.testError
	})

	return success, errorMsg, nil
}

// CloseTestQueue releases plugin workers waiting for queued tests.
func (s *TestbedResourceServer) CloseTestQueue() {
	s.testResult.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.queuedTestsClosed = true
		broadcast()
	})
}

// FailQueuedTests records a global failure for all queued test waiters.
func (s *TestbedResourceServer) FailQueuedTests(success bool, errorMsg string) {
	s.testResult.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.testCompleted = true
		s.testSuccess = success
		s.testError = errorMsg

		if success {
			s.le.Info("test marked as successful")
		} else {
			s.le.Errorf("test marked as failed: %s", errorMsg)
		}

		broadcast()
	})
}

func (s *TestbedResourceServer) markQueuedTestResult(testName string, success bool, errorMsg string) {
	s.testResult.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if s.queuedTestResults == nil {
			s.queuedTestResults = make(map[string]queuedTestResult)
		}
		s.queuedTestResults[testName] = queuedTestResult{
			success:  success,
			errorMsg: errorMsg,
		}

		if success {
			s.le.Infof("test %s marked as successful", testName)
		} else {
			s.le.Errorf("test %s marked as failed: %s", testName, errorMsg)
		}

		broadcast()
	})
}

// WaitForTestResult waits for the legacy test result.
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
