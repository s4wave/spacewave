//go:build !js

package resource_world_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// TestWorldStateResourceTypedListPaging checks distinct, stable pages through
// the generated RPC client and the real World Resource service.
func TestWorldStateResourceTypedListPaging(t *testing.T) {
	// Populate a World larger than the maximum typed page.
	ctx := t.Context()
	tb, cleanup := setupWorldTestbed(ctx, t)
	t.Cleanup(cleanup)
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)

	// Create the type shared by the matching subjects.
	const typeID = "test/paged"
	if err := world_types.EnsureTypeExists(ctx, tx, typeID); err != nil {
		t.Fatal(err)
	}
	want := make(map[string]bool, world_types.ObjectTypePageLimit+3)
	for i := range int(world_types.ObjectTypePageLimit+3) * 2 {
		// Create keys in reverse lexical order, interleaving unrelated objects.
		key := "paged/" + strconv.Itoa(10000-i)
		obj, err := tx.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if i%2 != 0 {
			continue
		}

		// Add several type quads for one subject to exercise deduplication.
		want[key] = true
		for _, label := range []string{"", world_types.BuildTypeObjectKey(typeID)} {
			q := world.NewGraphQuadWithKeys(key, world_types.TypePred.String(), world_types.BuildTypeObjectKey(typeID), label)
			if err := tx.SetGraphQuad(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Mount one unchanged snapshot on the real World Resource service.
	read, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(read.Discard)
	resource := resource_world.NewMount(nil, tb.Bus, nil).NewWorldStateResource(read)
	client := s4wave_world.NewSRPCWorldStateResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(resource.GetMux()))))

	// Compare page sizes to ensure the cursor preserves the same subject order.
	var order []string
	for _, limit := range []uint32{17, world_types.ObjectTypePageLimit, ^uint32(0)} {
		// Walk this page size, rejecting repeated or unrelated keys.
		keys := make([]string, 0, len(want))
		seen := make(map[string]bool, len(want))
		var after string
		var pages int
		for {
			// Read one bounded page and verify its distinct matching subjects.
			resp, err := client.ListObjectsWithType(ctx, &s4wave_world.ListObjectsWithTypeRequest{
				TypeId: typeID, StartAfter: after, Limit: limit,
			})
			if err != nil {
				t.Fatal(err)
			}
			page := resp.GetObjectKeys()
			if len(page) == 0 || len(page) > int(min(limit, world_types.ObjectTypePageLimit)) {
				t.Fatalf("invalid page length %d for limit %d", len(page), limit)
			}
			for _, key := range page {
				if !want[key] || seen[key] {
					t.Fatalf("unrelated or repeated key %q", key)
				}
				seen[key] = true
			}

			// Retain the ordered result and resume only when more subjects remain.
			keys = append(keys, page...)
			pages++
			if !resp.GetMore() {
				break
			}
			after = page[len(page)-1]
		}

		// Require every matching key and stable order across page boundaries.
		if len(keys) != len(want) || pages < 2 {
			t.Fatalf("got %d keys in %d pages, want %d keys in multiple pages", len(keys), pages, len(want))
		}
		if order == nil {
			order = keys
			continue
		}
		if !slices.Equal(keys, order) {
			t.Fatal("subject order changed with page size")
		}
	}

	// The unchanged generic helpers traverse all pages in the same order.
	keys, err := world_types.ListObjectsWithType(ctx, read, typeID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(keys, order) {
		t.Fatal("generic typed listing differs from paged RPC listing")
	}
	var visited []string
	err = world_types.IterateObjectsWithType(ctx, read, typeID, func(key string) (bool, error) {
		visited = append(visited, key)
		return true, nil
	})
	if err != nil || !slices.Equal(visited, order) {
		t.Fatalf("typed traversal differs from paged RPC listing: %v", err)
	}

	// Reject an unbounded request and return an empty page for an absent type.
	if _, err := client.ListObjectsWithType(ctx, &s4wave_world.ListObjectsWithTypeRequest{TypeId: typeID}); err == nil {
		t.Fatal("accepted zero limit")
	}
	empty, err := client.ListObjectsWithType(ctx, &s4wave_world.ListObjectsWithTypeRequest{TypeId: "test/absent", Limit: 17})
	if err != nil || len(empty.GetObjectKeys()) != 0 || empty.GetMore() {
		t.Fatalf("absent type returned %v, %v", empty, err)
	}
}
