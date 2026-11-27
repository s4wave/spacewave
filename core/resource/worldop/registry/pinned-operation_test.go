package resource_worldop_registry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/hash"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/sirupsen/logrus"
)

// TestPinnedOperationReplay uses retained executable identity without a current registration.
func TestPinnedOperationReplay(t *testing.T) {
	// Bound the whole replay scenario with a test timeout.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	le := logrus.NewEntry(logrus.New())

	// Hash the old executable identity that the pinned operation retains.
	oldHash, err := hash.Sum(hash.RecommendedHashType, []byte("old executable"))
	if err != nil {
		t.Fatal(err)
	}

	// The current runtime implements different behavior. Replay must never
	// call that runtime.
	client := func(key string) srpc.Client {
		return handlerClient(t, &pinnedTestHandler{key: key})
	}

	// Hash an executable identity whose root the host proved has no build here.
	foreignHash, err := hash.Sum(hash.RecommendedHashType, []byte("foreign executable"))
	if err != nil {
		t.Fatal(err)
	}

	// Register a plugin loader whose latest and historical clients differ.
	load := &testWorldOpPluginLoadController{
		client:    client("replay/latest"),
		manifests: map[string]srpc.Client{oldHash.MarshalString(): client("replay/original")},
		missing:   map[string]bool{foreignHash.MarshalString(): true},
	}

	// Pin a root whose worker fails its connection only as the deadline ends.
	hangHash, err := hash.Sum(hash.RecommendedHashType, []byte("hanging executable"))
	if err != nil {
		t.Fatal(err)
	}
	load.manifests[hangHash.MarshalString()] = deadlineFailClient{}

	// Pin a root whose worker fails the apply call at once, as one that
	// crashes mid-call does, and one whose worker refuses every attach.
	crashHash, err := hash.Sum(hash.RecommendedHashType, []byte("crashing executable"))
	if err != nil {
		t.Fatal(err)
	}
	load.manifests[crashHash.MarshalString()] = handlerClient(t, &pinnedTestHandler{key: "replay/crash", applyErr: "worker crashed"})
	attachHash, err := hash.Sum(hash.RecommendedHashType, []byte("attach failing executable"))
	if err != nil {
		t.Fatal(err)
	}
	load.manifests[attachHash.MarshalString()] = attachFailClient{Client: client("replay/attach")}

	// Add the plugin load controller to the testbed bus.
	release, err := tb.Bus.AddController(ctx, load, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Add the registry bridge controller that routes world operations.
	bridge := NewWorldOpRegistryBridgeController(le, tb.Bus, NewWorldOpRegistryResource(nil))
	releaseBridge, err := tb.Bus.AddController(ctx, bridge, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseBridge()

	// Resolve the identifier retained in history, then apply it in caller-owned state.

	// Resolve the pinned operation identifier and acquire its lookup reference.
	id := registry.PinnedOperationID("test-plugin", oldHash.MarshalString(), "colors/like")
	lookups, _, ref, err := world.ExLookupWorldOp(ctx, tb.Bus, le, id, tb.EngineID)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	if len(lookups) != 1 {
		t.Fatalf("operation lookups = %d", len(lookups))
	}

	// Materialize the operation and verify it kept its immutable identity.
	op, err := lookups[0](ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if op.TypeID() != id {
		t.Fatal("operation lost its immutable identity")
	}

	// Apply the pinned operation inside a caller-owned write transaction.
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if _, err := op.ApplyWorldOp(ctx, le, tx, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertWorldObjectExists(t, ctx, tb.Engine, "replay/original")

	// Open a read transaction to verify the latest implementation never ran.
	read, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	if found, err := read.HasObject(ctx, "replay/latest"); err != nil || found {
		t.Fatalf("latest implementation ran: found=%t, err=%v", found, err)
	}

	// Removing an artifact produces an explicit failure even while the
	// latest executable is available.

	// Hash an executable identity that has no registered manifest.
	missingHash, err := hash.Sum(hash.RecommendedHashType, []byte("missing executable"))
	if err != nil {
		t.Fatal(err)
	}

	// Build a bridge operation pinned to the missing executable hash.
	missing := newBridgeOperation(le, tb.Bus, &registry.WorldOpRegistration{PluginId: "test-plugin"}, id, tb.EngineID, pluginLoadDeadline)
	missing.manifestRoot = missingHash.MarshalString()
	missing.handlerID = "colors/like"

	// An executable that never loads is not proven missing: the apply may pass later.
	_, err = missing.ApplyWorldOp(ctx, le, read, tb.Volume.GetPeerID())
	var unavailable *world.PluginUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("unloaded executable error = %v, expected PluginUnavailableError", err)
	}

	// A proven missing build is final for this device and reports the
	// pinned root.
	foreign := newBridgeOperation(le, tb.Bus, &registry.WorldOpRegistration{PluginId: "test-plugin"}, id, tb.EngineID, pluginLoadDeadline)
	foreign.manifestRoot = foreignHash.MarshalString()
	foreign.handlerID = "colors/like"
	_, err = foreign.ApplyWorldOp(ctx, le, read, tb.Volume.GetPeerID())
	var executableMissing *world.PluginExecutableMissingError
	if !errors.As(err, &executableMissing) || executableMissing.ManifestRoot != foreign.manifestRoot {
		t.Fatalf("foreign executable error = %v, expected PluginExecutableMissingError", err)
	}
	var rejection *world.OperationRejection
	if errors.As(err, &rejection) {
		t.Fatalf("missing executable became a stored rejection: %v", err)
	}

	// A connect error that arrives at the operation's deadline is a wait that
	// replay retries, not a failure it records on this replica.
	hang := newBridgeOperation(le, tb.Bus, &registry.WorldOpRegistration{PluginId: "test-plugin"}, id, tb.EngineID, pluginLoadDeadline)
	hang.manifestRoot = hangHash.MarshalString()
	hang.handlerID = "colors/like"
	opCtx, opCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer opCancel()
	_, err = hang.ApplyWorldOp(opCtx, le, read, tb.Volume.GetPeerID())
	if !errors.As(err, &unavailable) {
		t.Fatalf("connect error at the deadline = %v, expected PluginUnavailableError", err)
	}

	// So is a handler error the worker returns as the deadline ends: the
	// call loses the error's type, and the deadline decides.
	<-opCtx.Done()
	if err := hang.callErr(opCtx, errors.New("handler failed"), true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("handler error at the deadline = %v, expected a deadline", err)
	}

	// A nested call can return its deadline as an untyped RPC error before
	// the outer timer publishes cancellation. The shared budget is already
	// spent even though the outer context still reports no error.
	budget := world.NewWriteBudget(time.Nanosecond)
	budget.Start()
	deadline, _ := budget.Deadline()
	passed, cancelPassed := context.WithDeadline(t.Context(), deadline)
	defer cancelPassed()
	<-passed.Done()
	outerCtx := world.WithWriteBudget(t.Context(), budget)
	deadlineErr := hang.callErr(outerCtx, errors.New("nested handler failed"), true)
	timeout, ok := errors.AsType[*world.PluginOpTimeoutError](deadlineErr)
	if !ok || timeout.Deadline != budget.GetLength() || outerCtx.Err() != nil {
		t.Fatalf("nested error at the shared deadline = %v, outer context = %v", deadlineErr, outerCtx.Err())
	}

	// A handler that fails its call long before the deadline gave no verdict,
	// so replay waits at it as at an unreachable plugin: the error is the type
	// replay counts as a gap, marked sent, and never a stored rejection.
	crash := newBridgeOperation(le, tb.Bus, &registry.WorldOpRegistration{PluginId: "test-plugin"}, id, tb.EngineID, pluginLoadDeadline)
	crash.manifestRoot = crashHash.MarshalString()
	crash.handlerID = "colors/like"
	liveCtx, liveCancel := context.WithTimeout(ctx, 10*time.Second)
	defer liveCancel()
	_, err = crash.ApplyWorldOp(liveCtx, le, read, tb.Volume.GetPeerID())
	if !errors.As(err, &unavailable) || !unavailable.Sent || errors.As(err, &rejection) || liveCtx.Err() != nil {
		t.Fatalf("handler failure before the deadline = %v, expected a sent PluginUnavailableError", err)
	}

	// So is an attach the worker refuses, before the handler was asked.
	attach := newBridgeOperation(le, tb.Bus, &registry.WorldOpRegistration{PluginId: "test-plugin"}, id, tb.EngineID, pluginLoadDeadline)
	attach.manifestRoot = attachHash.MarshalString()
	attach.handlerID = "colors/like"
	_, err = attach.ApplyWorldOp(liveCtx, le, read, tb.Volume.GetPeerID())
	if !errors.As(err, &unavailable) || unavailable.Sent || errors.As(err, &rejection) || liveCtx.Err() != nil {
		t.Fatalf("attach failure = %v, expected an unsent PluginUnavailableError", err)
	}
}

// handlerClient serves handler behind a resource server over a client pipe.
func handlerClient(t *testing.T, handler registry.SRPCWorldOpHandlerServiceServer) srpc.Client {
	// Expose the handler behind a resource server mux.
	root := srpc.NewMux()
	if err := registry.SRPCRegisterWorldOpHandlerService(root, handler); err != nil {
		t.Fatal(err)
	}

	// Wrap the handler mux with the resource server and a client pipe.
	mux := srpc.NewMux()
	if err := resource_server.NewResourceServer(root).Register(mux); err != nil {
		t.Fatal(err)
	}
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
}

// attachFailClient is a worker connection that refuses every resource attach
// with a plain error and serves every other call.
type attachFailClient struct {
	srpc.Client
}

// NewStream fails a resource attach and passes every other stream on.
func (c attachFailClient) NewStream(ctx context.Context, service, method string, first srpc.Message) (srpc.Stream, error) {
	if method == "ResourceAttach" {
		return nil, errors.New("attach refused")
	}
	return c.Client.NewStream(ctx, service, method, first)
}

// deadlineFailClient is a worker connection that answers nothing and fails
// with a plain error once its call's context ends, as a dropped transport does.
type deadlineFailClient struct{}

// ExecCall fails with a plain error once ctx ends.
func (deadlineFailClient) ExecCall(ctx context.Context, _, _ string, _, _ srpc.Message) error {
	<-ctx.Done()
	return errors.New("worker connection reset")
}

// NewStream fails with a plain error once ctx ends.
func (deadlineFailClient) NewStream(ctx context.Context, _, _ string, _ srpc.Message) (srpc.Stream, error) {
	<-ctx.Done()
	return nil, errors.New("worker connection reset")
}

// pinnedTestHandler gives each executable a distinguishable observable effect.
type pinnedTestHandler struct {
	key string
	// applyErr, when set, fails every apply with this plain error.
	applyErr string
}

func (h *pinnedTestHandler) ValidateOp(_ context.Context, req *registry.ValidateOpRequest) (*registry.ValidateOpResponse, error) {
	if req.GetOperationTypeId() != "colors/like" {
		return nil, fmt.Errorf("handler received executable identity instead of local operation name")
	}
	return &registry.ValidateOpResponse{}, nil
}

func (h *pinnedTestHandler) ApplyWorldOp(ctx context.Context, req *registry.ApplyWorldOpRequest) (*registry.ApplyWorldOpResponse, error) {
	// Fail as a crashing worker does, when told to.
	if h.applyErr != "" {
		return nil, errors.New(h.applyErr)
	}

	// Get the resource client context carrying the attached resource client.
	call, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Attach the requested world state resource for this handler call.
	client, err := call.GetAttachedResource(req.GetAttachedWorldStateResourceId())
	if err != nil {
		return nil, err
	}

	// Create the object keyed by this handler's identity and release it.
	state := sdk_world.NewSRPCWorldStateResourceServiceClient(client)
	object, err := state.CreateObject(ctx, &sdk_world.CreateObjectRequest{ObjectKey: h.key})
	if err != nil {
		return nil, err
	}
	call.ReleaseResource(object.GetResourceId())
	return &registry.ApplyWorldOpResponse{}, nil
}

func (h *pinnedTestHandler) ApplyWorldObjectOp(context.Context, *registry.ApplyWorldObjectOpRequest) (*registry.ApplyWorldObjectOpResponse, error) {
	return nil, fmt.Errorf("unexpected object operation")
}
