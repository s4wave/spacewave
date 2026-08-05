//go:build !js

package resource_world_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// setupWorldTestbed creates a hydra world testbed and returns it.
func setupWorldTestbed(ctx context.Context, t *testing.T) (*world_testbed.Testbed, func()) {
	// Create the World testbed and release callback.
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	cleanup := func() {
		tb.Release()
	}

	return tb, cleanup
}

// setupWorldResourceClient sets up resource client and returns Engine SDK wrapper.
func setupWorldResourceClient(ctx context.Context, t *testing.T, tb *world_testbed.Testbed) (*resource_client.Client, *s4wave_world.Engine, func()) {
	// Acquire the resource client and create a World engine.
	resClient, clientCleanup := resource_testbed.SetupResourceClient(ctx, t, tb)

	rootRef := resClient.AccessRootResource()

	srpcClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		clientCleanup()
		t.Fatal(err.Error())
	}

	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createWorldResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		rootRef.Release()
		clientCleanup()
		t.Fatal(err.Error())
	}

	// Build the engine wrapper and cleanup closure.
	engineRef := resClient.CreateResourceReference(createWorldResp.ResourceId)
	engine, err := s4wave_world.NewEngine(resClient, engineRef)
	if err != nil {
		rootRef.Release()
		clientCleanup()
		t.Fatal(err.Error())
	}

	cleanup := func() {
		engine.Release()
		rootRef.Release()
		clientCleanup()
	}

	return resClient, engine, cleanup
}

// TestGraphPathQueryResourceClose tests path query resource paging and close behavior.
func TestGraphPathQueryResourceClose(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
	defer cleanup()

	// Create graph objects and commit their edges.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Release()

	for _, key := range []string{"query/a", "query/b", "query/c"} {
		if _, err := tx.CreateObject(ctx, key, nil); err != nil {
			t.Fatal(err.Error())
		}
	}
	for _, edge := range [][2]string{
		{"query/a", "query/b"},
		{"query/a", "query/c"},
	} {
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(edge[0], "<query-rel>", edge[1], "")); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Open a read transaction and execute the graph-path query.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Release()

	srpcClient, err := readTx.GetResourceRef().GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	worldService := s4wave_world.NewSRPCWorldStateResourceServiceClient(srpcClient)
	resp, err := worldService.QueryGraphPath(ctx, &s4wave_world.QueryGraphPathRequest{
		StartKeys: []string{"query/a"},
		Steps: []*s4wave_world.GraphPathStep{
			{
				Direction: s4wave_world.GraphPathDirection_GRAPH_PATH_DIRECTION_OUT,
				Predicate: "<query-rel>",
				Limit:     10,
			},
		},
		ResultLimit: 10,
		PageSize:    1,
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Page and close the query resource.
	queryRef := resClient.CreateResourceReference(resp.GetResourceId())
	defer queryRef.Release()
	queryClient, err := queryRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	queryService := s4wave_world.NewSRPCGraphPathQueryResourceServiceClient(queryClient)
	page, err := queryService.Next(ctx, &s4wave_world.NextGraphPathQueryRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(page.GetObjectKeys()) != 1 || page.GetDone() {
		t.Fatalf("expected one non-terminal page, got keys=%#v done=%v", page.GetObjectKeys(), page.GetDone())
	}
	if _, err := queryService.Close(ctx, &s4wave_world.CloseGraphPathQueryRequest{}); err != nil {
		t.Fatal(err.Error())
	}
	page, err = queryService.Next(ctx, &s4wave_world.NextGraphPathQueryRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if !page.GetDone() || len(page.GetObjectKeys()) != 0 {
		t.Fatalf("expected closed query to return done, got keys=%#v done=%v", page.GetObjectKeys(), page.GetDone())
	}

	// Verify canceled and closed resource behavior.
	resource := resource_world.NewGraphPathQueryResource(nil, nil, &world.GraphPathQueryResult{
		ObjectKeys: []string{"query/direct"},
	}, 1)
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := resource.Next(cancelCtx, &s4wave_world.NextGraphPathQueryRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled Next to return context.Canceled, got %v", err)
	}
	if _, err := resource.Close(ctx, &s4wave_world.CloseGraphPathQueryRequest{}); err != nil {
		t.Fatal(err.Error())
	}
}

func TestWorldStateListGraphEdgeBuckets(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
	defer cleanup()

	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Release()

	for _, key := range []string{
		"bucket/origin",
		"bucket/source-a",
		"bucket/source-b",
		"bucket/target-a",
		"bucket/target-b",
		"bucket/target-c",
	} {
		if _, err := tx.CreateObject(ctx, key, nil); err != nil {
			t.Fatal(err.Error())
		}
	}
	for _, edge := range [][3]string{
		{"bucket/origin", "<bucket-z>", "bucket/target-c"},
		{"bucket/origin", "<bucket-a>", "bucket/target-a"},
		{"bucket/origin", "<bucket-b>", "bucket/target-b"},
		{"bucket/source-b", "<bucket-in-b>", "bucket/origin"},
		{"bucket/source-a", "<bucket-in-a>", "bucket/origin"},
	} {
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(edge[0], edge[1], edge[2], "")); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Release()

	buckets, err := readTx.ListGraphEdgeBuckets(ctx, &world.GraphEdgeBucketQuery{
		OriginObjectKeys: []string{"bucket/origin"},
		LimitPerOrigin:   2,
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(buckets) != 1 {
		t.Fatalf("expected one bucket, got %d", len(buckets))
	}
	bucket := buckets[0]
	if bucket.OriginObjectKey != "bucket/origin" {
		t.Fatalf("expected origin bucket, got %q", bucket.OriginObjectKey)
	}
	if len(bucket.Outgoing) != 2 || len(bucket.Incoming) != 2 {
		t.Fatalf("expected limited outgoing/incoming buckets, got outgoing=%d incoming=%d", len(bucket.Outgoing), len(bucket.Incoming))
	}
	if !bucket.OutgoingTruncated || bucket.IncomingTruncated {
		t.Fatalf("unexpected truncation outgoing=%v incoming=%v", bucket.OutgoingTruncated, bucket.IncomingTruncated)
	}
	if got := bucket.Outgoing[0].GetPredicate(); got != "<bucket-a>" {
		t.Fatalf("expected sorted outgoing first predicate <bucket-a>, got %q", got)
	}
	if got := bucket.Incoming[0].GetSubject(); got != "<bucket/source-a>" {
		t.Fatalf("expected sorted incoming first subject <bucket/source-a>, got %q", got)
	}
}

func TestGraphPathQueryResourceReturnsQuadsWithoutObjectKeys(t *testing.T) {
	ctx := context.Background()
	resource := resource_world.NewGraphPathQueryResource(nil, nil, &world.GraphPathQueryResult{
		Quads: []world.GraphQuad{
			world.NewGraphQuadWithKeys("query/a", "<query-rel>", "query/b", ""),
		},
	}, 1)

	page, err := resource.Next(ctx, &s4wave_world.NextGraphPathQueryRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if !page.GetDone() {
		t.Fatal("expected terminal page")
	}
	if len(page.GetObjectKeys()) != 0 {
		t.Fatalf("expected no object keys, got %#v", page.GetObjectKeys())
	}
	if len(page.GetQuads()) != 1 {
		t.Fatalf("expected traversed quad on terminal empty-key page, got %d", len(page.GetQuads()))
	}

	page, err = resource.Next(ctx, &s4wave_world.NextGraphPathQueryRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if !page.GetDone() || len(page.GetQuads()) != 0 {
		t.Fatalf("expected already drained terminal page, got done=%v quads=%d", page.GetDone(), len(page.GetQuads()))
	}
}

func TestWorldStateResourceOperationObserverLookupGraphQuadsBatch(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	for _, key := range []string{"operation/a", "operation/b", "operation/c"} {
		if _, err := tb.WorldState.CreateObject(ctx, key, nil); err != nil {
			t.Fatal(err.Error())
		}
	}
	for _, edge := range [][2]string{
		{"operation/a", "operation/c"},
		{"operation/b", "operation/c"},
	} {
		if err := tb.WorldState.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(edge[0], "<operation-rel>", edge[1], "")); err != nil {
			t.Fatal(err.Error())
		}
	}

	var records []resource_world.WorldStateOperationRecord
	resource := resource_world.NewWorldStateResource(nil, nil, tb.WorldState, nil, resource_world.WithWorldStateOperationObserver(func(record resource_world.WorldStateOperationRecord) {
		records = append(records, record)
	}))
	subjFilter := world.NewGraphQuadWithKeys("operation/a", "<operation-rel>", "", "")
	objFilter := world.NewGraphQuadWithKeys("", "<operation-rel>", "operation/c", "")
	resp, err := resource.LookupGraphQuadsBatch(ctx, &s4wave_world.LookupGraphQuadsBatchRequest{
		Filters: []*quad.Quad{
			{Subject: subjFilter.GetSubject(), Predicate: subjFilter.GetPredicate()},
			{Predicate: objFilter.GetPredicate(), Obj: objFilter.GetObj()},
		},
		LimitPerFilter: 10,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(resp.GetResults()) != 2 {
		t.Fatalf("expected two result sets, got %d", len(resp.GetResults()))
	}
	if len(records) != 1 {
		t.Fatalf("expected one operation record, got %d", len(records))
	}
	record := records[0]
	if record.Name != "LookupGraphQuadsBatch" {
		t.Fatalf("record name = %q", record.Name)
	}
	if record.FilterCount != 2 || record.Limit != 10 {
		t.Fatalf("unexpected request counts: %+v", record)
	}
	if record.ResultSetCount != 2 || record.ResultQuadCount != 3 {
		t.Fatalf("unexpected result counts: %+v", record)
	}
	if record.Duration <= 0 {
		t.Fatalf("expected positive duration, got %s", record.Duration)
	}
	if record.Error != "" {
		t.Fatalf("unexpected error record: %+v", record)
	}

	records = nil
	if _, err := resource.LookupGraphQuadsBatch(ctx, &s4wave_world.LookupGraphQuadsBatchRequest{
		Filters:        []*quad.Quad{{Subject: subjFilter.GetSubject(), Predicate: subjFilter.GetPredicate()}},
		LimitPerFilter: 0,
	}); err == nil {
		t.Fatal("expected zero limit to fail")
	}
	if len(records) != 1 {
		t.Fatalf("expected one error operation record, got %d", len(records))
	}
	record = records[0]
	if record.Name != "LookupGraphQuadsBatch" || record.Error == "" {
		t.Fatalf("unexpected error record: %+v", record)
	}
	if record.FilterCount != 1 || record.Limit != 0 || record.ResultQuadCount != 0 {
		t.Fatalf("unexpected error counts: %+v", record)
	}
}

func TestWorldStateResourceGetObjectRootRefsBatch(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	alphaRef := &bucket.ObjectRef{BucketId: "alpha-bucket"}
	betaRef := &bucket.ObjectRef{BucketId: "beta-bucket"}
	if _, err := tb.WorldState.CreateObject(ctx, "root-ref/alpha", alphaRef); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := tb.WorldState.CreateObject(ctx, "root-ref/beta", betaRef); err != nil {
		t.Fatal(err.Error())
	}

	var records []resource_world.WorldStateOperationRecord
	resource := resource_world.NewWorldStateResource(nil, nil, tb.WorldState, nil, resource_world.WithWorldStateOperationObserver(func(record resource_world.WorldStateOperationRecord) {
		records = append(records, record)
	}))
	resp, err := resource.GetObjectRootRefsBatch(ctx, &s4wave_world.GetObjectRootRefsBatchRequest{
		ObjectKeys: []string{"root-ref/beta", "missing", "root-ref/alpha", "root-ref/alpha"},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	refs := resp.GetRootRefs()
	if len(refs) != 4 {
		t.Fatalf("expected 4 root refs, got %d", len(refs))
	}
	checkRootRef := func(ref *s4wave_world.ObjectRootRef, key string, exists bool, bucketID string) {
		if ref.GetObjectKey() != key || ref.GetExists() != exists {
			t.Fatalf("unexpected root ref for %s: %+v", key, ref)
		}
		if !exists {
			if ref.GetRootRef() != nil || ref.GetRev() != 0 {
				t.Fatalf("expected missing root ref for %s to be empty: %+v", key, ref)
			}
			return
		}
		if ref.GetRootRef().GetBucketId() != bucketID || ref.GetRev() != 1 {
			t.Fatalf("unexpected root ref for %s: %+v", key, ref)
		}
	}
	checkRootRef(refs[0], "root-ref/beta", true, "beta-bucket")
	checkRootRef(refs[1], "missing", false, "")
	checkRootRef(refs[2], "root-ref/alpha", true, "alpha-bucket")
	checkRootRef(refs[3], "root-ref/alpha", true, "alpha-bucket")

	if len(records) != 1 {
		t.Fatalf("expected one operation record, got %d", len(records))
	}
	record := records[0]
	if record.Name != "GetObjectRootRefsBatch" {
		t.Fatalf("record name = %q", record.Name)
	}
	if record.StartKeyCount != 4 || record.ResultObjectCount != 3 {
		t.Fatalf("unexpected object counts: %+v", record)
	}
	if record.Duration <= 0 {
		t.Fatalf("expected positive duration, got %s", record.Duration)
	}
}

func TestWorldStateResourceGetObjectBodiesBatch(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	for _, entry := range []struct {
		key string
		msg string
	}{
		{key: "body/alpha", msg: "alpha"},
		{key: "body/beta", msg: "beta"},
		{key: "body/gamma", msg: "gamma"},
	} {
		_, _, err := world.CreateWorldObject(ctx, tb.WorldState, entry.key, func(bcs *block.Cursor) error {
			bcs.SetBlock(block_mock.NewExample(entry.msg), true)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	var records []resource_world.WorldStateOperationRecord
	resource := resource_world.NewWorldStateResource(nil, nil, tb.WorldState, nil, resource_world.WithWorldStateOperationObserver(func(record resource_world.WorldStateOperationRecord) {
		records = append(records, record)
	}))
	resp, err := resource.GetObjectBodiesBatch(ctx, &s4wave_world.GetObjectBodiesBatchRequest{
		ObjectKeys: []string{"body/gamma", "body/missing", "body/alpha", "body/alpha"},
	})
	if err != nil {
		t.Fatal(err)
	}

	bodies := resp.GetBodies()
	if len(bodies) != 4 {
		t.Fatalf("expected 4 bodies, got %d", len(bodies))
	}
	checkBody := func(index int, key string, msg string, exists bool) {
		t.Helper()
		body := bodies[index]
		if body.GetObjectKey() != key || body.GetExists() != exists {
			t.Fatalf("body %d = key %q exists %v, want key %q exists %v", index, body.GetObjectKey(), body.GetExists(), key, exists)
		}
		if !exists {
			if body.GetBody() != nil {
				t.Fatalf("body %d has data for missing key", index)
			}
			return
		}
		decoded := block_mock.NewExample("")
		if err := decoded.UnmarshalBlock(body.GetBody()); err != nil {
			t.Fatalf("decode body %d: %v", index, err)
		}
		if decoded.GetMsg() != msg {
			t.Fatalf("body %d message = %q, want %q", index, decoded.GetMsg(), msg)
		}
	}
	checkBody(0, "body/gamma", "gamma", true)
	checkBody(1, "body/missing", "", false)
	checkBody(2, "body/alpha", "alpha", true)
	checkBody(3, "body/alpha", "alpha", true)

	if len(records) != 1 {
		t.Fatalf("expected one operation record, got %d", len(records))
	}
	record := records[0]
	if record.Name != "GetObjectBodiesBatch" {
		t.Fatalf("record name = %q", record.Name)
	}
	if record.StartKeyCount != 4 || record.ResultObjectCount != 3 {
		t.Fatalf("unexpected object counts: %+v", record)
	}
}

func TestWorldStateResourceLookupGraphQuadsBatchUsesOwnerOperation(t *testing.T) {
	ctx := context.Background()

	graphQuad := world.NewGraphQuadWithKeys("owner-batch/a", "<owner-batch-rel>", "owner-batch/b", "")
	ws := &worldStateOwnerBatchTestState{
		results: [][]world.GraphQuad{{graphQuad}},
	}
	resource := resource_world.NewWorldStateResource(nil, nil, ws, nil)
	resp, err := resource.LookupGraphQuadsBatch(ctx, &s4wave_world.LookupGraphQuadsBatchRequest{
		Filters: []*quad.Quad{
			{
				Subject:   graphQuad.GetSubject(),
				Predicate: graphQuad.GetPredicate(),
			},
		},
		LimitPerFilter: 10,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if ws.lookupCalls != 0 {
		t.Fatalf("expected resource to use owner batch operation, got %d primitive lookups", ws.lookupCalls)
	}
	if ws.batchCalls != 1 {
		t.Fatalf("expected one owner batch call, got %d", ws.batchCalls)
	}
	if ws.limit != 10 || len(ws.filters) != 1 || ws.filters[0].GetSubject() != graphQuad.GetSubject() {
		t.Fatalf("unexpected owner batch request: limit=%d filters=%#v", ws.limit, ws.filters)
	}
	if len(resp.GetResults()) != 1 || len(resp.GetResults()[0].GetQuads()) != 1 {
		t.Fatalf("unexpected owner batch response: %#v", resp.GetResults())
	}
}

func TestWorldStateResourceOperationObserverQueryGraphPath(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	for _, key := range []string{"operation-path/a", "operation-path/b", "operation-path/c"} {
		if _, err := tb.WorldState.CreateObject(ctx, key, nil); err != nil {
			t.Fatal(err.Error())
		}
	}
	for _, edge := range [][2]string{
		{"operation-path/a", "operation-path/b"},
		{"operation-path/b", "operation-path/c"},
	} {
		if err := tb.WorldState.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(edge[0], "<operation-path-rel>", edge[1], "")); err != nil {
			t.Fatal(err.Error())
		}
	}

	var records []resource_world.WorldStateOperationRecord
	resource := resource_world.NewWorldStateResource(nil, nil, tb.WorldState, nil, resource_world.WithWorldStateOperationObserver(func(record resource_world.WorldStateOperationRecord) {
		records = append(records, record)
	}))
	resourceCtx := &worldStateOperationResourceContext{ctx: ctx}
	resp, err := resource.QueryGraphPath(resource_server.WithResourceClientContext(ctx, resourceCtx), &s4wave_world.QueryGraphPathRequest{
		StartKeys: []string{"operation-path/a"},
		Steps: []*s4wave_world.GraphPathStep{
			{
				Direction: s4wave_world.GraphPathDirection_GRAPH_PATH_DIRECTION_OUT,
				Predicate: "<operation-path-rel>",
				Limit:     10,
			},
			{
				Direction: s4wave_world.GraphPathDirection_GRAPH_PATH_DIRECTION_OUT,
				Predicate: "<operation-path-rel>",
				Limit:     10,
			},
		},
		ResultLimit:  10,
		IncludeQuads: true,
		PageSize:     1,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetResourceId() != 1 {
		t.Fatalf("resource id = %d, want 1", resp.GetResourceId())
	}
	if len(records) != 1 {
		t.Fatalf("expected one operation record, got %d", len(records))
	}
	record := records[0]
	if record.Name != "QueryGraphPath" {
		t.Fatalf("record name = %q", record.Name)
	}
	if record.StartKeyCount != 1 || record.StepCount != 2 || record.Limit != 10 || record.PageSize != 1 {
		t.Fatalf("unexpected request counts: %+v", record)
	}
	if record.ResultObjectCount != 1 || record.ResultQuadCount != 2 || !record.ResourceCreated {
		t.Fatalf("unexpected result counts: %+v", record)
	}
	if record.Duration <= 0 {
		t.Fatalf("expected positive duration, got %s", record.Duration)
	}
	if record.Error != "" {
		t.Fatalf("unexpected error record: %+v", record)
	}
	resourceCtx.ReleaseResource(resp.GetResourceId())
}

func TestEngineResourceOperationObserverPropagatesToTransactions(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	var records []resource_world.WorldStateOperationRecord
	engineResource := resource_world.NewEngineResource(
		nil,
		nil,
		tb.Engine,
		nil,
		nil,
		resource_world.WithWorldStateOperationObserver(func(record resource_world.WorldStateOperationRecord) {
			records = append(records, record)
		}),
	)
	resourceCtx := &worldStateOperationResourceContext{ctx: ctx}
	ctx = resource_server.WithResourceClientContext(ctx, resourceCtx)

	writeResp, err := engineResource.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: true})
	if err != nil {
		t.Fatal(err.Error())
	}
	writeClient, err := resourceCtx.GetAttachedResource(writeResp.GetResourceId())
	if err != nil {
		t.Fatal(err.Error())
	}
	writeWorld := s4wave_world.NewSRPCWorldStateResourceServiceClient(writeClient)
	writeTx := s4wave_world.NewSRPCTxResourceServiceClient(writeClient)
	for _, key := range []string{"engine-observer/a", "engine-observer/b"} {
		resp, err := writeWorld.CreateObject(ctx, &s4wave_world.CreateObjectRequest{ObjectKey: key})
		if err != nil {
			t.Fatal(err.Error())
		}
		resourceCtx.ReleaseResource(resp.GetResourceId())
	}
	graphQuad := world.NewGraphQuadWithKeys("engine-observer/a", "<engine-observer-rel>", "engine-observer/b", "")
	if _, err := writeWorld.SetGraphQuad(ctx, &s4wave_world.SetGraphQuadRequest{
		Quad: &quad.Quad{
			Subject:   graphQuad.GetSubject(),
			Predicate: graphQuad.GetPredicate(),
			Obj:       graphQuad.GetObj(),
		},
	}); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := writeTx.Commit(ctx, &s4wave_world.CommitRequest{}); err != nil {
		t.Fatal(err.Error())
	}
	resourceCtx.ReleaseResource(writeResp.GetResourceId())

	readResp, err := engineResource.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer resourceCtx.ReleaseResource(readResp.GetResourceId())

	readClient, err := resourceCtx.GetAttachedResource(readResp.GetResourceId())
	if err != nil {
		t.Fatal(err.Error())
	}
	readWorld := s4wave_world.NewSRPCWorldStateResourceServiceClient(readClient)
	resp, err := readWorld.LookupGraphQuadsBatch(ctx, &s4wave_world.LookupGraphQuadsBatchRequest{
		Filters: []*quad.Quad{
			{
				Subject:   graphQuad.GetSubject(),
				Predicate: graphQuad.GetPredicate(),
			},
		},
		LimitPerFilter: 10,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(resp.GetResults()) != 1 || len(resp.GetResults()[0].GetQuads()) != 1 {
		t.Fatalf("expected one propagated transaction result, got %#v", resp.GetResults())
	}
	if len(records) != 1 {
		t.Fatalf("expected one operation record, got %d", len(records))
	}
	record := records[0]
	if record.Name != "LookupGraphQuadsBatch" || record.FilterCount != 1 || record.Limit != 10 {
		t.Fatalf("unexpected propagated record request counts: %+v", record)
	}
	if record.ResultSetCount != 1 || record.ResultQuadCount != 1 || record.Error != "" {
		t.Fatalf("unexpected propagated record result counts: %+v", record)
	}
}

type worldStateOperationResourceContext struct {
	ctx      context.Context
	nextID   uint32
	releases map[uint32]func()
	invokers map[uint32]srpc.Invoker
}

func (c *worldStateOperationResourceContext) Context() context.Context {
	return c.ctx
}

func (c *worldStateOperationResourceContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

func (c *worldStateOperationResourceContext) AddResourceValue(mux srpc.Invoker, _ any, releaseFn func()) (uint32, error) {
	c.nextID++
	if c.releases == nil {
		c.releases = make(map[uint32]func())
	}
	if c.invokers == nil {
		c.invokers = make(map[uint32]srpc.Invoker)
	}
	if releaseFn == nil {
		releaseFn = func() {}
	}
	c.releases[c.nextID] = releaseFn
	c.invokers[c.nextID] = mux
	return c.nextID, nil
}

func (c *worldStateOperationResourceContext) ReleaseResource(resourceID uint32) bool {
	releaseFn, ok := c.releases[resourceID]
	if !ok {
		return false
	}
	delete(c.releases, resourceID)
	delete(c.invokers, resourceID)
	releaseFn()
	return true
}

func (c *worldStateOperationResourceContext) GetResourceValue(uint32) (any, error) {
	return nil, resource.ErrResourceNotFound
}

func (c *worldStateOperationResourceContext) GetAttachedResource(resourceID uint32) (srpc.Client, error) {
	mux, ok := c.invokers[resourceID]
	if !ok {
		return nil, resource.ErrResourceNotFound
	}
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))), nil
}

// _ is a type assertion
var _ resource_server.ResourceClientContext = ((*worldStateOperationResourceContext)(nil))

type worldStateOwnerBatchTestState struct {
	world.WorldState

	filters     []world.GraphQuad
	limit       uint32
	results     [][]world.GraphQuad
	lookupCalls int
	batchCalls  int
}

func (s *worldStateOwnerBatchTestState) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	s.lookupCalls++
	return nil, nil
}

func (s *worldStateOwnerBatchTestState) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	s.batchCalls++
	s.filters = filters
	s.limit = limitPerFilter
	return s.results, nil
}

// TestWorldStateBasicOperations tests basic WorldState operations using the SDK.
func TestWorldStateBasicOperations(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	t.Run("CreateAndGetObject", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		objKey := "test-object-" + t.Name()
		rootRef := &bucket.ObjectRef{}

		obj, err := tx.CreateObject(ctx, objKey, rootRef)
		if err != nil {
			t.Fatalf("CreateObject failed: %v", err)
		}

		key := obj.GetKey()
		if key != objKey {
			t.Fatalf("expected key %q, got %q", objKey, key)
		}

		retrievedObj, found, err := tx.GetObject(ctx, objKey)
		if err != nil {
			t.Fatalf("GetObject failed: %v", err)
		}
		if !found {
			t.Fatal("object not found")
		}

		retrievedKey := retrievedObj.GetKey()
		if retrievedKey != objKey {
			t.Fatalf("expected retrieved key %q, got %q", objKey, retrievedKey)
		}

		t.Logf("Successfully created and retrieved object with key: %s", objKey)
	})

	t.Run("GetNonexistentObject", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		obj, found, err := tx.GetObject(ctx, "nonexistent-key")
		if err != nil {
			t.Fatalf("GetObject failed: %v", err)
		}
		if found {
			t.Fatal("expected object not found, but found=true")
		}
		if obj != nil {
			t.Fatal("expected nil object for not found")
		}

		t.Log("Correctly returned not found for nonexistent object")
	})

	t.Run("DeleteObject", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		objKey := "test-delete-" + t.Name()
		rootRef := &bucket.ObjectRef{}

		_, err = tx.CreateObject(ctx, objKey, rootRef)
		if err != nil {
			t.Fatalf("CreateObject failed: %v", err)
		}

		deleted, err := tx.DeleteObject(ctx, objKey)
		if err != nil {
			t.Fatalf("DeleteObject failed: %v", err)
		}
		if !deleted {
			t.Fatal("expected deleted=true")
		}

		_, found, err := tx.GetObject(ctx, objKey)
		if err != nil {
			t.Fatalf("GetObject after delete failed: %v", err)
		}
		if found {
			t.Fatal("expected object not found after delete")
		}

		t.Logf("Successfully deleted object: %s", objKey)
	})

	t.Run("GetReadOnly", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		readOnly := tx.GetReadOnly()
		if readOnly {
			t.Fatal("expected read-write transaction, got read-only")
		}

		t.Log("Correctly returned read-only status")
	})

	t.Run("GetSeqno", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		seqno, err := tx.GetSeqno(ctx)
		if err != nil {
			t.Fatalf("GetSeqno failed: %v", err)
		}

		t.Logf("Current seqno: %d", seqno)
	})

	t.Run("BuildStorageCursorOnTx", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		cursorResourceId, err := tx.BuildStorageCursor(ctx)
		if err != nil {
			t.Fatalf("BuildStorageCursor failed: %v", err)
		}

		if cursorResourceId == 0 {
			t.Fatal("expected non-zero cursor resource ID")
		}

		cursorRef := resClient.CreateResourceReference(cursorResourceId)
		defer cursorRef.Release()

		t.Logf("Successfully built storage cursor from transaction, resource_id: %d", cursorResourceId)
	})

	t.Run("BuildStorageCursorOnEngine", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		cursorResourceId, err := engine.BuildStorageCursor(ctx)
		if err != nil {
			t.Fatalf("BuildStorageCursor failed: %v", err)
		}

		if cursorResourceId == 0 {
			t.Fatal("expected non-zero cursor resource ID")
		}

		cursorRef := resClient.CreateResourceReference(cursorResourceId)
		defer cursorRef.Release()

		t.Logf("Successfully built storage cursor from engine, resource_id: %d", cursorResourceId)
	})

	t.Run("AccessWorldStateOnTx", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		cursorResourceId, err := tx.AccessWorldState(ctx, nil)
		if err != nil {
			t.Fatalf("AccessWorldState failed: %v", err)
		}

		if cursorResourceId == 0 {
			t.Fatal("expected non-zero cursor resource ID")
		}

		cursorRef := resClient.CreateResourceReference(cursorResourceId)
		defer cursorRef.Release()

		t.Logf("Successfully accessed world state from transaction, resource_id: %d", cursorResourceId)
	})

	t.Run("AccessWorldStateOnEngine", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		cursorResourceId, err := engine.AccessWorldState(ctx, nil)
		if err != nil {
			t.Fatalf("AccessWorldState failed: %v", err)
		}

		if cursorResourceId == 0 {
			t.Fatal("expected non-zero cursor resource ID")
		}

		cursorRef := resClient.CreateResourceReference(cursorResourceId)
		defer cursorRef.Release()

		t.Logf("Successfully accessed world state from engine, resource_id: %d", cursorResourceId)
	})
}

func TestEngineWorldRootSnapshots(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
	defer cleanup()

	initial, err := engine.GetWorldRootSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetWorldRootSnapshot failed: %v", err)
	}
	if initial.GetRootRef().GetBucketId() == "" {
		t.Fatalf("initial root bucket id is empty: %#v", initial.GetRootRef())
	}
	if initial.GetEngineInfo().GetEngineId() == "" || initial.GetEngineInfo().GetBucketId() == "" {
		t.Fatalf("missing engine info: %#v", initial.GetEngineInfo())
	}
	if initial.GetEngineInfo().GetBucketId() != initial.GetRootRef().GetBucketId() {
		t.Fatalf("engine bucket %q differs from root bucket %q", initial.GetEngineInfo().GetBucketId(), initial.GetRootRef().GetBucketId())
	}
	if initial.GetStorageVolumeId() == "" {
		t.Fatal("storage volume id is empty")
	}

	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(read) failed: %v", err)
	}
	readTx.Release()
	afterReadOnly, err := engine.GetWorldRootSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetWorldRootSnapshot after read-only tx failed: %v", err)
	}
	if afterReadOnly.GetSeqno() != initial.GetSeqno() || !afterReadOnly.GetRootRef().EqualsRef(initial.GetRootRef()) {
		t.Fatalf("read-only transaction changed root: before=%#v after=%#v", initial, afterReadOnly)
	}

	stream, err := engine.WatchWorldRootSnapshots(ctx)
	if err != nil {
		t.Fatalf("WatchWorldRootSnapshots failed: %v", err)
	}
	watchedInitial, err := stream.Recv()
	if err != nil {
		t.Fatalf("initial root snapshot recv failed: %v", err)
	}
	if watchedInitial.GetSeqno() != initial.GetSeqno() || !watchedInitial.GetRootRef().EqualsRef(initial.GetRootRef()) {
		t.Fatalf("initial stream snapshot mismatch: get=%#v watch=%#v", initial, watchedInitial)
	}

	writeTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(write) failed: %v", err)
	}
	obj, err := writeTx.CreateObject(ctx, "root-snapshot/object", &bucket.ObjectRef{})
	if err != nil {
		t.Fatalf("CreateObject failed: %v", err)
	}
	if _, err := obj.IncrementRev(ctx); err != nil {
		t.Fatalf("IncrementRev failed: %v", err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	writeTx.Release()

	next, err := stream.Recv()
	if err != nil {
		t.Fatalf("next root snapshot recv failed: %v", err)
	}
	if next.GetSeqno() <= initial.GetSeqno() {
		t.Fatalf("root snapshot seqno did not advance: before=%d after=%d", initial.GetSeqno(), next.GetSeqno())
	}
	if next.GetRootRef().EqualsRef(initial.GetRootRef()) {
		t.Fatalf("root snapshot ref did not advance: before=%#v after=%#v", initial.GetRootRef(), next.GetRootRef())
	}

	current, err := engine.GetWorldRootSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetWorldRootSnapshot after write failed: %v", err)
	}
	if current.GetSeqno() != next.GetSeqno() || !current.GetRootRef().EqualsRef(next.GetRootRef()) {
		t.Fatalf("get/watch root mismatch: get=%#v watch=%#v", current, next)
	}
}

// TestWatchWorldState tests the reactive WorldState watch functionality.
func TestWatchWorldState(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	t.Run("ReceivesInitialResourceId", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		stream, err := engine.WatchWorldState(ctx)
		if err != nil {
			t.Fatalf("WatchWorldState failed: %v", err)
		}

		msg, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv failed: %v", err)
		}

		if msg.ResourceId == 0 {
			t.Fatal("expected non-zero resource_id")
		}

		t.Logf("Received initial resource_id: %d", msg.ResourceId)
	})

	t.Run("DetectsObjectChanges", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		stream, err := engine.WatchWorldState(ctx)
		if err != nil {
			t.Fatalf("WatchWorldState failed: %v", err)
		}

		initialMsg, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv failed: %v", err)
		}
		t.Logf("Initial resource_id: %d", initialMsg.ResourceId)

		// Get tracked WorldState to access
		trackedRef := resClient.CreateResourceReference(initialMsg.ResourceId)
		defer trackedRef.Release()

		trackedWs, err := s4wave_world.NewWorldState(resClient, trackedRef, false)
		if err != nil {
			t.Fatalf("NewWorldState failed: %v", err)
		}

		// Access an object through tracked WorldState to register tracking
		objKey := "test-watch-object-" + t.Name()
		_, found, err := trackedWs.GetObject(ctx, objKey)
		if err != nil {
			t.Fatalf("GetObject failed: %v", err)
		}
		if found {
			t.Fatal("expected object not found initially")
		}

		// Create a NEW write transaction for making changes
		writeTx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction for write failed: %v", err)
		}
		defer writeTx.Release()

		// Make changes through write transaction
		obj, err := writeTx.CreateObject(ctx, objKey, &bucket.ObjectRef{})
		if err != nil {
			t.Fatalf("CreateObject failed: %v", err)
		}

		_, err = obj.IncrementRev(ctx)
		if err != nil {
			t.Fatalf("IncrementRev failed: %v", err)
		}

		// Commit the write transaction
		err = writeTx.Commit(ctx)
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Watch should detect changes
		changeMsg, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv after change failed: %v", err)
		}

		if changeMsg.ResourceId == initialMsg.ResourceId {
			t.Fatal("expected different resource_id after change")
		}

		t.Logf("Detected change - new resource_id: %d", changeMsg.ResourceId)
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		watchCtx, watchCancel := context.WithCancel(ctx)

		stream, err := engine.WatchWorldState(watchCtx)
		if err != nil {
			t.Fatalf("WatchWorldState failed: %v", err)
		}

		_, err = stream.Recv()
		if err != nil {
			t.Fatalf("Recv failed: %v", err)
		}

		watchCancel()

		_, err = stream.Recv()
		if err == nil {
			t.Fatal("expected error after context cancellation")
		}
		if err != io.EOF && err != context.Canceled {
			t.Logf("Got expected error: %v", err)
		}

		t.Log("Correctly handled context cancellation")
	})

	t.Run("UniqueResourceIds", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClient(ctx, t, tb)
		defer cleanup()

		stream, err := engine.WatchWorldState(ctx)
		if err != nil {
			t.Fatalf("WatchWorldState failed: %v", err)
		}

		msg1, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv failed: %v", err)
		}

		if msg1.ResourceId == 0 {
			t.Fatal("expected non-zero initial resource_id")
		}
		t.Logf("Received initial resource_id: %d", msg1.ResourceId)

		seenIds := map[uint32]bool{msg1.ResourceId: true}

		trackedRef := resClient.CreateResourceReference(msg1.ResourceId)
		defer trackedRef.Release()
		trackedWs, err := s4wave_world.NewWorldState(resClient, trackedRef, false)
		if err != nil {
			t.Fatalf("NewWorldState failed: %v", err)
		}

		for i := range 3 {
			objKey := fmt.Sprintf("test-unique-%s-%d", t.Name(), i)

			// Access object through tracked WorldState to register tracking
			_, _, err := trackedWs.GetObject(ctx, objKey)
			if err != nil {
				t.Fatalf("GetObject failed: %v", err)
			}

			// Create a NEW write transaction for each change
			writeTx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				t.Fatalf("NewTransaction for write failed: %v", err)
			}

			obj, err := writeTx.CreateObject(ctx, objKey, &bucket.ObjectRef{})
			if err != nil {
				writeTx.Release()
				t.Fatalf("CreateObject failed: %v", err)
			}

			_, err = obj.IncrementRev(ctx)
			if err != nil {
				writeTx.Release()
				t.Fatalf("IncrementRev failed: %v", err)
			}

			// Commit the write transaction
			err = writeTx.Commit(ctx)
			if err != nil {
				writeTx.Release()
				t.Fatalf("Commit failed: %v", err)
			}
			writeTx.Release()

			msg, err := stream.Recv()
			if err != nil {
				t.Fatalf("Recv failed: %v", err)
			}

			if seenIds[msg.ResourceId] {
				t.Fatalf("duplicate resource_id: %d", msg.ResourceId)
			}
			seenIds[msg.ResourceId] = true
			t.Logf("Received unique resource_id %d after change %d", msg.ResourceId, i+1)

			trackedRef.Release()
			trackedRef = resClient.CreateResourceReference(msg.ResourceId)
			trackedWs, err = s4wave_world.NewWorldState(resClient, trackedRef, false)
			if err != nil {
				t.Fatalf("NewWorldState failed: %v", err)
			}
		}

		if len(seenIds) != 4 {
			t.Fatalf("expected 4 unique resource_ids, got %d", len(seenIds))
		}

		t.Log("Successfully received unique resource IDs for each change")
	})
}
