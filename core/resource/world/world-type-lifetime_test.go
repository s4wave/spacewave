//go:build !js

package resource_world_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// TestWorldTypeOperationsReleaseResources checks server ownership while the
// client remains connected across repeated existence checks and type creation.
func TestWorldTypeOperationsReleaseResources(t *testing.T) {
	for _, mode := range []string{"exists", "types"} {
		t.Run(mode, func(t *testing.T) {
			// Open the production SDK over the existing resource testbed.
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			t.Cleanup(cancel)
			tb, err := world_testbed.Default(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tb.Release)
			client, server, cleanup := setupCountingResourceClient(ctx, t, tb)
			t.Cleanup(cleanup)
			root := client.AccessRootResource()
			t.Cleanup(root.Release)
			rpc, err := root.GetClient()
			if err != nil {
				t.Fatal(err)
			}
			created, err := s4wave_testbed.NewSRPCTestbedResourceServiceClient(rpc).CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
			if err != nil {
				t.Fatal(err)
			}
			ref := client.CreateResourceReference(created.GetResourceId())
			t.Cleanup(ref.Release)
			engine, err := sdk_world_engine.NewSDKEngine(client, ref)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.GetSeqno(ctx); err != nil {
				t.Fatal(err)
			}
			baseline := server.CountTrackedResources()

			// Commit each operation, retaining only the root and engine resources.
			for i := range 25 {
				err := world.ExecTransaction(ctx, engine, true, func(ctx context.Context, ws world.WorldState) error {
					key := "resource-types/" + strconv.Itoa(i)
					if mode == "types" {
						if _, err := world_types.EnsureTypeExists(ctx, ws, key); err != nil {
							return err
						}
						_, err := world_types.EnsureTypeExists(ctx, ws, key)
						return err
					}
					obj, err := ws.CreateObject(ctx, key, nil)
					world.ReleaseObjectState(obj)
					if err != nil {
						return err
					}
					found, err := ws.HasObject(ctx, key)
					if err != nil {
						return err
					}
					if !found {
						t.Error("created object is missing")
					}
					found, err = ws.HasObject(ctx, key+"/missing")
					if found {
						t.Error("absent object exists")
					}
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := engine.GetSeqno(ctx); err != nil {
					t.Fatal(err)
				}
				if got := server.CountTrackedResources(); got != baseline {
					t.Fatalf("after transaction %d: retained %d server resources, want %d", i+1, got, baseline)
				}
			}
		})
	}
}
