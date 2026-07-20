//go:build !js && !windows

package resource_testbed_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
)

func TestRemoteObjectStateApplyObjectOpPreservesExecutionClaim(t *testing.T) {
	if os.Getenv(remoteClaimHelperEnv) == "1" {
		serveRemoteClaimTestbed(t)
		return
	}

	ctx := context.Background()
	resClient, cleanup := newCrossProcessResourceClient(t, ctx)
	t.Cleanup(cleanup)

	rootRef := resClient.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	const engineID = "remote-object-op-claim"
	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(rootClient)
	createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{EngineId: engineID})
	if err != nil {
		t.Fatal(err)
	}
	engineRef := resClient.CreateResourceReference(createResp.GetResourceId())
	engine, err := sdk_world_engine.NewSDKEngine(resClient, engineRef)
	if err != nil {
		engineRef.Release()
		t.Fatal(err)
	}
	t.Cleanup(engine.Release)

	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	const objectKey = "test/execution/remote-claim"
	if _, _, err := world.AccessWorldObject(ctx, world.NewEngineWorldState(engine, true), objectKey, true, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(&forge_execution.Execution{
			ExecutionState: forge_execution.State_ExecutionState_PENDING,
			PeerId:         peerID.String(),
			Timestamp:      timestamp.Now(),
			ValueSet:       forge_target.NewValueSet(),
		}, true)
		bcs.FollowRef(4, nil).SetBlock(&forge_target.Target{
			Exec: &forge_target.Exec{Disable: true},
		}, true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	obj, found, err := tx.GetObject(ctx, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("execution object not found through remote transaction")
	}
	defer world.ReleaseObjectState(obj)
	const claimID = "remote-claim-owner"
	if _, _, err := obj.ApplyObjectOp(ctx, execution_tx.NewTxStart(peerID, claimID), peerID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := world.ExecTransaction(ctx, engine, false, func(ctx context.Context, ws world.WorldState) error {
		execution, _, err := forge_execution.LookupExecution(ctx, ws, objectKey)
		if err != nil {
			return err
		}
		if got := execution.GetExecutionState(); got != forge_execution.State_ExecutionState_RUNNING {
			t.Fatalf("execution state = %s, want RUNNING", got)
		}
		claim := execution.GetClaim()
		if claim == nil {
			t.Fatal("execution claim was dropped across remote ObjectState.ApplyObjectOp")
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

const remoteClaimHelperEnv = "SPACEWAVE_REMOTE_CLAIM_HELPER"

func closeRemoteClaimTestResource(t *testing.T, name string, resource io.Closer) {
	t.Helper()
	if err := resource.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Errorf("close %s: %v", name, err)
	}
}

func removeRemoteClaimTestDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Errorf("remove remote claim test directory: %v", err)
	}
}

func killRemoteClaimTestHelper(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if err := cmd.Process.Kill(); err != nil {
		t.Errorf("kill remote claim helper: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Error("remote claim helper exited cleanly after kill")
	}
}

func newCrossProcessResourceClient(
	t *testing.T,
	ctx context.Context,
) (*resource_client.Client, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("/var/tmp", "swc-")
	if err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(dir, "resource.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		removeRemoteClaimTestDir(t, dir)
		t.Fatal(err)
	}
	unixListener, ok := listener.(*net.UnixListener)
	if !ok {
		closeRemoteClaimTestResource(t, "unix listener", listener)
		removeRemoteClaimTestDir(t, dir)
		t.Fatal("unix listener has unexpected type")
	}
	unixListener.SetUnlinkOnClose(false)
	listenerFile, err := unixListener.File()
	if err != nil {
		closeRemoteClaimTestResource(t, "unix listener", listener)
		removeRemoteClaimTestDir(t, dir)
		t.Fatal(err)
	}

	var output bytes.Buffer
	// #nosec G204 -- test re-execs its own binary with constant arguments
	cmd := exec.Command(os.Args[0], "-test.run=^TestRemoteObjectStateApplyObjectOpPreservesExecutionClaim$", "-test.v")
	cmd.Env = append(os.Environ(), remoteClaimHelperEnv+"=1")
	cmd.ExtraFiles = []*os.File{listenerFile}
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		closeRemoteClaimTestResource(t, "listener file", listenerFile)
		closeRemoteClaimTestResource(t, "unix listener", listener)
		removeRemoteClaimTestDir(t, dir)
		t.Fatal(err)
	}
	closeRemoteClaimTestResource(t, "listener file", listenerFile)
	closeRemoteClaimTestResource(t, "unix listener", listener)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		killRemoteClaimTestHelper(t, cmd)
		removeRemoteClaimTestDir(t, dir)
		t.Fatalf("dial helper: %v\n%s", err, output.String())
	}
	clientMux, err := srpc.NewMuxedConn(conn, true, nil)
	if err != nil {
		closeRemoteClaimTestResource(t, "client connection", conn)
		killRemoteClaimTestHelper(t, cmd)
		removeRemoteClaimTestDir(t, dir)
		t.Fatal(err)
	}
	client := srpc.NewClientWithMuxedConn(clientMux)
	resources, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(client))
	if err != nil {
		closeRemoteClaimTestResource(t, "client connection", conn)
		killRemoteClaimTestHelper(t, cmd)
		removeRemoteClaimTestDir(t, dir)
		t.Fatal(err)
	}

	return resources, func() {
		resources.Release()
		closeRemoteClaimTestResource(t, "client connection", conn)
		if err := cmd.Wait(); err != nil {
			t.Errorf("remote claim helper: %v\n%s", err, output.String())
		}
		removeRemoteClaimTestDir(t, dir)
	}
}

func serveRemoteClaimTestbed(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	listenerFile := os.NewFile(3, "remote-claim-listener")
	if listenerFile == nil {
		t.Fatal("remote claim listener file is missing")
	}
	listener, err := net.FileListener(listenerFile)
	closeRemoteClaimTestResource(t, "inherited listener file", listenerFile)
	if err != nil {
		t.Fatal(err)
	}
	defer closeRemoteClaimTestResource(t, "helper listener", listener)
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer closeRemoteClaimTestResource(t, "helper connection", conn)

	logger := logrus.New()
	root := resource_testbed.NewTestbedResourceServer(
		ctx,
		logrus.NewEntry(logger),
		tb.Bus,
		tb.Volume.GetID(),
		tb.BucketId,
	)
	mux := srpc.NewMux()
	resourceServer := resource_server.NewResourceServer(root.GetMux())
	if err := resourceServer.Register(mux); err != nil {
		t.Fatal(err)
	}
	serverMux, err := srpc.NewMuxedConn(conn, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := srpc.NewServer(mux)
	if err := server.AcceptMuxedConn(ctx, serverMux); err != nil &&
		!errors.Is(err, io.EOF) &&
		!errors.Is(err, io.ErrClosedPipe) &&
		!errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}
