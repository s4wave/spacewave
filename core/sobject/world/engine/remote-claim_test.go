package sobject_world_engine_test

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/s4wave/spacewave/testbed"
)

func TestRemoteSharedObjectWorldApplyPreservesExecutionClaim(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))

	peerID := tb.Volume.GetPeerID()
	_, providerRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     peerID.String(),
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(providerRef.Release)
	_, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	provRef.Release()
	account, accountRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, "local", "remote-claim", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(accountRef.Release)
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, account)
	if err != nil {
		t.Fatal(err)
	}
	soRef, err := soProvider.CreateSharedObject(ctx, "remote-claim", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}

	const engineID = "remote-shared-object-claim"
	engineController, _, engineControllerRef, err := sobject_world_engine.StartEngineWithConfig(
		ctx,
		tb.Bus,
		sobject_world_engine.NewConfig(engineID, soRef),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engineControllerRef.Release)
	opController := world.NewLookupOpController("remote-execution-ops", engineID, execution_tx.LookupWorldOp)
	releaseOps, err := tb.Bus.AddController(ctx, opController, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseOps)
	engine, err := engineController.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}

	const objectKey = "test/execution/remote-shared-object-claim"
	if err := world.ExecTransaction(ctx, engine, true, func(ctx context.Context, ws world.WorldState) error {
		_, err := forge_execution.CreateExecutionWithTarget(
			ctx,
			ws,
			peerID,
			objectKey,
			peerID,
			forge_target.NewValueSet(),
			&forge_target.Target{Exec: &forge_target.Exec{Disable: true}},
			timestamp.Now(),
		)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	remote, cleanup := newRemoteSharedObjectEngine(t, ctx, tb, engine)
	t.Cleanup(cleanup)
	if err := world.ExecTransaction(ctx, remote, false, func(ctx context.Context, ws world.WorldState) error {
		execution, _, err := forge_execution.LookupExecution(ctx, ws, objectKey)
		if err != nil {
			return err
		}
		if got := execution.GetExecutionState(); got != forge_execution.State_ExecutionState_PENDING {
			t.Fatalf("initial execution state = %s, want PENDING", got)
		}
		if execution.GetClaim() != nil {
			t.Fatal("initial execution unexpectedly has a claim")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	const claimID = "remote-shared-object-owner"
	if err := world.ExecTransaction(ctx, remote, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := world.MustGetObject(ctx, ws, objectKey)
		if err != nil {
			return err
		}
		defer world.ReleaseObjectState(obj)
		_, _, err = obj.ApplyObjectOp(ctx, execution_tx.NewTxStart(peerID, claimID), peerID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := world.ExecTransaction(ctx, remote, false, func(ctx context.Context, ws world.WorldState) error {
		execution, _, err := forge_execution.LookupExecution(ctx, ws, objectKey)
		if err != nil {
			return err
		}
		if got := execution.GetExecutionState(); got != forge_execution.State_ExecutionState_RUNNING {
			t.Fatalf("execution state = %s, want RUNNING", got)
		}
		claim := execution.GetClaim()
		if claim == nil {
			t.Fatal("execution claim was dropped across the remote SharedObject World boundary")
		}
		if got := claim.GetClaimId(); got != claimID {
			t.Fatalf("execution claim ID = %q, want %q", got, claimID)
		}
		if got := claim.GetEpoch(); got != 1 {
			t.Fatalf("execution claim epoch = %d, want 1", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func closeRemoteSharedObjectTestResource(t *testing.T, name string, resource io.Closer) {
	t.Helper()
	if err := resource.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Errorf("close %s: %v", name, err)
	}
}

func newRemoteSharedObjectEngine(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	engine world.Engine,
) (*sdk_world_engine.SDKEngine, func()) {
	t.Helper()
	clientPipe, serverPipe := net.Pipe()
	clientMux, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		closeRemoteSharedObjectTestResource(t, "client pipe", clientPipe)
		closeRemoteSharedObjectTestResource(t, "server pipe", serverPipe)
		t.Fatal(err)
	}
	serverMuxed, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		closeRemoteSharedObjectTestResource(t, "client pipe", clientPipe)
		closeRemoteSharedObjectTestResource(t, "server pipe", serverPipe)
		t.Fatal(err)
	}
	engineResource := resource_world.NewEngineResource(
		tb.Logger,
		tb.Bus,
		engine,
		execution_tx.LookupWorldOp,
		nil,
	)
	mux := srpc.NewMux()
	resourceServer := resource_server.NewResourceServer(engineResource.GetMux())
	if err := resourceServer.Register(mux); err != nil {
		closeRemoteSharedObjectTestResource(t, "client pipe", clientPipe)
		closeRemoteSharedObjectTestResource(t, "server pipe", serverPipe)
		t.Fatal(err)
	}
	server := srpc.NewServer(mux)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.AcceptMuxedConn(ctx, serverMuxed)
	}()
	stopServer := func() {
		closeRemoteSharedObjectTestResource(t, "client pipe", clientPipe)
		closeRemoteSharedObjectTestResource(t, "server pipe", serverPipe)
		if err := <-serverErr; err != nil &&
			!errors.Is(err, io.EOF) &&
			!errors.Is(err, io.ErrClosedPipe) &&
			!errors.Is(err, net.ErrClosed) {
			t.Errorf("remote SharedObject server: %v", err)
		}
	}
	client := srpc.NewClientWithMuxedConn(clientMux)
	resources, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(client))
	if err != nil {
		stopServer()
		t.Fatal(err)
	}
	rootRef := resources.AccessRootResource()
	remote, err := sdk_world_engine.NewSDKEngine(resources, rootRef)
	if err != nil {
		rootRef.Release()
		resources.Release()
		<-resources.Done()
		stopServer()
		t.Fatal(err)
	}
	cleanup := func() {
		remote.Release()
		resources.Release()
		<-resources.Done()
		stopServer()
	}
	return remote, cleanup
}
