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
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	le := logrus.NewEntry(logrus.New())
	oldHash, err := hash.Sum(hash.RecommendedHashType, []byte("old executable"))
	if err != nil {
		t.Fatal(err)
	}

	// The current runtime implements different behavior; replay must never call it.
	client := func(key string) srpc.Client {
		root := srpc.NewMux()
		if err := registry.SRPCRegisterWorldOpHandlerService(root, &pinnedTestHandler{key: key}); err != nil {
			t.Fatal(err)
		}
		mux := srpc.NewMux()
		if err := resource_server.NewResourceServer(root).Register(mux); err != nil {
			t.Fatal(err)
		}
		return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	}
	load := &testWorldOpPluginLoadController{
		client:    client("replay/latest"),
		manifests: map[string]srpc.Client{oldHash.MarshalString(): client("replay/original")},
	}
	release, err := tb.Bus.AddController(ctx, load, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	bridge := NewWorldOpRegistryBridgeController(le, tb.Bus, NewWorldOpRegistryResource(nil))
	releaseBridge, err := tb.Bus.AddController(ctx, bridge, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseBridge()

	// Resolve the identifier retained in history, then apply it in caller-owned state.
	id := registry.PinnedOperationID("test-plugin", oldHash.MarshalString(), "colors/like")
	lookups, _, ref, err := world.ExLookupWorldOp(ctx, tb.Bus, le, id, tb.EngineID)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	if len(lookups) != 1 {
		t.Fatalf("operation lookups = %d", len(lookups))
	}
	op, err := lookups[0](ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if op.GetOperationTypeId() != id {
		t.Fatal("operation lost its immutable identity")
	}
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
	read, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	if found, err := read.HasObject(ctx, "replay/latest"); err != nil || found {
		t.Fatalf("latest implementation ran: found=%t, err=%v", found, err)
	}

	// Removing an artifact produces an explicit failure even while latest is available.
	missingHash, err := hash.Sum(hash.RecommendedHashType, []byte("missing executable"))
	if err != nil {
		t.Fatal(err)
	}
	missing := newBridgeOperation(le, tb.Bus, &registry.WorldOpRegistration{PluginId: "test-plugin"}, id, tb.EngineID)
	missing.manifestRoot = missingHash.MarshalString()
	missing.handlerID = "colors/like"
	_, err = missing.ApplyWorldOp(ctx, le, read, tb.Volume.GetPeerID())
	var rejection *world.OperationRejection
	if !errors.As(err, &rejection) || rejection.Code != "UNAVAILABLE" {
		t.Fatalf("missing executable error = %v, expected UNAVAILABLE", err)
	}
}

// pinnedTestHandler gives each executable a distinguishable observable effect.
type pinnedTestHandler struct {
	key string
}

func (h *pinnedTestHandler) ValidateOp(_ context.Context, req *registry.ValidateOpRequest) (*registry.ValidateOpResponse, error) {
	if req.GetOperationTypeId() != "colors/like" {
		return nil, fmt.Errorf("handler received executable identity instead of local operation name")
	}
	return &registry.ValidateOpResponse{}, nil
}

func (h *pinnedTestHandler) ApplyWorldOp(ctx context.Context, req *registry.ApplyWorldOpRequest) (*registry.ApplyWorldOpResponse, error) {
	call, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	client, err := call.GetAttachedResource(req.GetAttachedWorldStateResourceId())
	if err != nil {
		return nil, err
	}
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
