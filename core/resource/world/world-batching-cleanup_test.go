//go:build !js && !wasip1

package resource_world_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	block_mock "github.com/s4wave/spacewave/db/block/mock"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	sdk_cursor "github.com/s4wave/spacewave/sdk/bucket/lookup"
	sdk_testbed "github.com/s4wave/spacewave/sdk/testbed"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
)

// Count real owner-side Resource handles, not merely local goroutines or a mock
// cursor count. Successful commits and discarded staged attempts must release
// every transaction, object, and cursor handle back to the same baseline.
func TestWorldBatchingResourceCleanupSyncedBolt(t *testing.T) {
	ctx := t.Context()
	tb, err := world_testbed.WithTestbedOptions(ctx, []db_testbed.Option{db_testbed.WithVolumeConfig(&volume_bolt.Config{
		Path: filepath.Join(t.TempDir(), "resources.bolt"), VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
	})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	client, server, cleanup := setupCountingResourceClient(ctx, t, tb)
	defer cleanup()
	root := client.AccessRootResource()
	defer root.Release()
	rpc, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := sdk_testbed.NewSRPCTestbedResourceServiceClient(rpc).CreateWorld(ctx, &sdk_testbed.CreateWorldRequest{EngineId: "batching-cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	ref := client.CreateResourceReference(resp.ResourceId)
	eng, err := sdk_world.NewEngine(client, ref)
	if err != nil {
		ref.Release()
		t.Fatal(err)
	}
	defer eng.Release()
	if _, err := eng.GetSeqno(ctx); err != nil {
		t.Fatal(err)
	}
	baseline := server.CountTrackedResources()
	for i := range 12 {
		w, err := eng.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		id, err := w.BuildStorageCursor(ctx)
		if err != nil {
			w.Release()
			t.Fatal(err)
		}
		err = sdk_cursor.AccessCursor(ctx, client, id, func(c *bucket_lookup.Cursor) error {
			tx, bcs := c.BuildTransaction(nil)
			bcs.SetBlock(block_mock.NewExample(fmt.Sprintf("retained body %d", i)), true)
			r, _, err := tx.Write(ctx, true)
			if err != nil {
				return err
			}
			ref := c.GetRef().Clone()
			ref.RootRef = r
			obj, err := w.CreateObject(ctx, fmt.Sprintf("object/%d", i), ref)
			world.ReleaseObjectState(obj)
			return err
		})
		if err == nil {
			if i%3 == 0 {
				err = w.Discard(ctx)
			} else {
				err = w.Commit(ctx)
			}
		}
		_ = w.Discard(context.Background())
		w.Release()
		if err != nil {
			t.Fatal(err)
		}
		waitForTrackedResourceCount(t, server, baseline)
	}
}
