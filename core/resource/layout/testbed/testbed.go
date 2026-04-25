//go:build !js

package layout_testbed

import (
	"context"
	"net"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/sirupsen/logrus"
)

// Testbed is a constructed layout testbed.
type Testbed struct {
	*world_testbed.Testbed

	// ResClient is the resource client for the testbed.
	ResClient *resource_client.Client
	// objectTypeCtrlRelease releases the ObjectType controller.
	objectTypeCtrlRelease func()
	// clientCleanup cleans up the resource client.
	clientCleanup func()
}

// NewTestbed constructs a new layout testbed from a world testbed.
func NewTestbed(ctx context.Context, tb *world_testbed.Testbed, opts ...Option) (t *Testbed, tbErr error) {
	if tb == nil {
		return nil, errors.New("testbed cannot be nil")
	}

	var rels []func()
	defer func() {
		if tbErr != nil {
			for _, r := range rels {
				r()
			}
		}
	}()

	for _, opt := range opts {
		switch opt.(type) {
		default:
			return nil, errors.Errorf("unrecognized testbed option: %#v", opt)
		}
	}

	// Register ObjectType controller with known types
	objectTypes := map[string]objecttype.ObjectType{
		s4wave_layout_world.ObjectLayoutTypeID: s4wave_layout_world.ObjectLayoutType,
	}
	lookupFunc := func(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
		return objectTypes[typeID], nil
	}
	objectTypeCtrl := objecttype_controller.NewController(lookupFunc)
	objectTypeCtrlRelease, err := tb.Bus.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		return nil, errors.Wrap(err, "add ObjectType controller")
	}
	rels = append(rels, objectTypeCtrlRelease)

	resClient, clientCleanup, err := setupResourceClient(ctx, tb)
	if err != nil {
		return nil, errors.Wrap(err, "setup resource client")
	}
	rels = append(rels, clientCleanup)

	return &Testbed{
		Testbed:               tb,
		ResClient:             resClient,
		objectTypeCtrlRelease: objectTypeCtrlRelease,
		clientCleanup:         clientCleanup,
	}, nil
}

// setupResourceClient creates pipes, muxed connections, and resource client.
func setupResourceClient(ctx context.Context, tb *world_testbed.Testbed) (*resource_client.Client, func(), error) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Create pipes for in-memory communication
	clientPipe, serverPipe := net.Pipe()

	// Create client muxed connection
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		return nil, nil, errors.Wrap(err, "create client muxed conn")
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)

	// Create server mux
	mux := srpc.NewMux()
	server := srpc.NewServer(mux)

	// Create TestbedResourceServer as root
	volumeID := tb.Volume.GetID()
	bucketID := tb.BucketId
	testbedResource := resource_testbed.NewTestbedResourceServer(ctx, le, tb.Bus, volumeID, bucketID)
	resourceServer := resource_server.NewResourceServer(testbedResource.GetMux())
	err = resourceServer.Register(mux)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		return nil, nil, errors.Wrap(err, "register resource server")
	}

	// Start server
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		return nil, nil, errors.Wrap(err, "create server muxed conn")
	}
	go func() {
		_ = server.AcceptMuxedConn(ctx, serverMp)
	}()

	// Create resource client
	resourceServiceClient := resource.NewSRPCResourceServiceClient(srpcClient)
	resClient, err := resource_client.NewClient(ctx, resourceServiceClient)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		return nil, nil, errors.Wrap(err, "create resource client")
	}

	cleanup := func() {
		resClient.Release()
		clientPipe.Close()
		serverPipe.Close()
	}

	return resClient, cleanup, nil
}

// Default constructs the default layout testbed arrangement.
func Default(ctx context.Context, opts ...Option) (*Testbed, error) {
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		return nil, err
	}
	tb2, err := NewTestbed(ctx, tb, opts...)
	if err != nil {
		tb.Release()
		return nil, err
	}
	return tb2, nil
}

// Release releases the testbed resources.
func (t *Testbed) Release() {
	if t.clientCleanup != nil {
		t.clientCleanup()
	}
	if t.objectTypeCtrlRelease != nil {
		t.objectTypeCtrlRelease()
	}
	t.Testbed.Release()
}

// Setup holds the test setup state for layout tests.
type Setup struct {
	// Testbed is the layout testbed.
	Testbed *Testbed
	// Engine is the world engine.
	Engine *s4wave_world.Engine
	// LayoutResourceID is the resource ID of the created layout.
	LayoutResourceID uint32
}

// SetupLayoutEngine creates an engine with ObjectLayout type registered and a demo layout created.
// Returns the setup struct containing engine, tx, and layout resource ID.
func (t *Testbed) SetupLayoutEngine(ctx context.Context, objectKey string) (*Setup, error) {
	rootRef := t.ResClient.AccessRootResource()

	srpcClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		return nil, errors.Wrap(err, "get SRPC client")
	}

	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createWorldResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		rootRef.Release()
		return nil, errors.Wrap(err, "create world")
	}

	engineRef := t.ResClient.CreateResourceReference(createWorldResp.ResourceId)
	engine, err := s4wave_world.NewEngine(t.ResClient, engineRef)
	if err != nil {
		rootRef.Release()
		return nil, errors.Wrap(err, "create engine")
	}

	// Create an ObjectLayout using the InitObjectLayoutOp
	sdkTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		engine.Release()
		rootRef.Release()
		return nil, errors.Wrap(err, "create transaction")
	}

	op := space_world_ops.NewInitObjectLayoutOp(objectKey, time.Now())
	opData, err := op.MarshalBlock()
	if err != nil {
		sdkTx.Release()
		engine.Release()
		rootRef.Release()
		return nil, errors.Wrap(err, "marshal block")
	}

	_, _, err = sdkTx.ApplyWorldOp(ctx, space_world_ops.InitObjectLayoutOpId, opData, "")
	if err != nil {
		sdkTx.Release()
		engine.Release()
		rootRef.Release()
		return nil, errors.Wrap(err, "apply world op")
	}

	err = sdkTx.Commit(ctx)
	if err != nil {
		sdkTx.Release()
		engine.Release()
		rootRef.Release()
		return nil, errors.Wrap(err, "commit")
	}
	sdkTx.Release()

	// Access the typed object to get the resource ID
	// Use a read transaction since we only need the resource ID
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		engine.Release()
		rootRef.Release()
		return nil, errors.Wrap(err, "create read transaction")
	}
	defer readTx.Release()

	txSrpcClient, err := readTx.GetResourceRef().GetClient()
	if err != nil {
		engine.Release()
		rootRef.Release()
		return nil, errors.Wrap(err, "get tx client")
	}

	typedSvcClient := s4wave_world.NewSRPCTypedObjectResourceServiceClient(txSrpcClient)
	resp, err := typedSvcClient.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
		ObjectKey: objectKey,
	})
	if err != nil {
		engine.Release()
		rootRef.Release()
		return nil, errors.Wrap(err, "access typed object")
	}

	return &Setup{
		Testbed:          t,
		Engine:           engine,
		LayoutResourceID: resp.ResourceId,
	}, nil
}

// Release releases the setup resources.
func (s *Setup) Release() {
	if s.Engine != nil {
		s.Engine.Release()
	}
}
