//go:build !js

package resource_world_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"

	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// errStopSearch is the client stopping a search stream.
var errStopSearch = errors.New("stop search")

// searchStream collects the pages a SearchObjectsWithType handler sends,
// stopping after stopAfter pages when stopAfter is set.
type searchStream struct {
	s4wave_world.SRPCWorldStateResourceService_SearchObjectsWithTypeStream
	ctx       context.Context
	stopAfter int
	pages     []*s4wave_world.SearchObjectsWithTypeResponse
}

// Context returns the stream context.
func (s *searchStream) Context() context.Context {
	return s.ctx
}

// Send records one page and stops the stream once it holds stopAfter pages.
func (s *searchStream) Send(resp *s4wave_world.SearchObjectsWithTypeResponse) error {
	s.pages = append(s.pages, resp)
	if s.stopAfter != 0 && len(s.pages) >= s.stopAfter {
		return errStopSearch
	}
	return nil
}

// TestSearchObjectsWithTypeFirstPage searches 10,000 objects of one type in
// each World format and checks, by counting block reads on a newly opened
// Engine, that a client reading only the first page costs a small fraction of
// a full search, and that the full search returns every match once.
func TestSearchObjectsWithTypeFirstPage(t *testing.T) {
	const n = 10000
	formats := []world_block.WorldFormat{
		world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV,
		world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH,
	}
	for _, format := range formats {
		t.Run(format.String(), func(t *testing.T) {
			ctx := t.Context()
			tbed, rootRef := seedSearchWorld(t, ctx, format, n)

			// Search with a client that stops after the first page.
			first, firstPages := searchColdEngine(t, ctx, tbed, rootRef, "OBJECT/", 1)
			if len(firstPages) != 1 || len(firstPages[0].GetObjectKeys()) != 100 {
				t.Fatalf("first page: %d pages", len(firstPages))
			}

			// Search to the end and check every object matched once.
			all, pages := searchColdEngine(t, ctx, tbed, rootRef, "OBJECT/", 0)
			seen := make(map[string]bool, n)
			for _, page := range pages {
				for _, key := range page.GetObjectKeys() {
					if seen[key] {
						t.Fatalf("repeated match %q", key)
					}
					seen[key] = true
				}
			}
			if len(pages) != n/100 || len(seen) != n {
				t.Fatalf("full search: %d pages, %d matches", len(pages), len(seen))
			}
			t.Logf("search reads at %d objects: first page %d blocks, full search %d blocks", n, first, all)
			if first*10 > all {
				t.Fatalf("first page read %d of %d blocks", first, all)
			}

			// A key query matches only its object.
			_, exact := searchColdEngine(t, ctx, tbed, rootRef, "object/9999", 0)
			if len(exact) != 1 || !slices.Equal(exact[0].GetObjectKeys(), []string{searchObjectKey(9999)}) {
				t.Fatalf("exact search: %v", exact)
			}
		})
	}
}

// searchColdEngine opens an Engine on rootRef, searches type search/a for
// query, and returns the blocks it read and the pages it sent.
func searchColdEngine(t *testing.T, ctx context.Context, tbed *testbed.Testbed, rootRef *bucket.ObjectRef, query string, stopAfter int) (uint64, []*s4wave_world.SearchObjectsWithTypeResponse) {
	t.Helper()

	// Open the Engine and a read transaction under a read counter.
	ctx, counter := block.WithReadCounter(ctx)
	ocs, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ocs.Release()
	ocs.SetRootRef(rootRef.GetRootRef())
	eng, err := world_block.NewEngine(ctx, logrus.NewEntry(logrus.New()), ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Run the search through the Resource handler.
	strm := &searchStream{ctx: ctx, stopAfter: stopAfter}
	err = resource_world.NewWorldStateResource(nil, nil, tx, nil).SearchObjectsWithType(&s4wave_world.SearchObjectsWithTypeRequest{TypeId: "search/a", Query: query}, strm)
	if err != nil && (stopAfter == 0 || !errors.Is(err, errStopSearch)) {
		t.Fatal(err)
	}
	return counter.Snapshot().BlockReadCount, strm.pages
}

// seedSearchWorld writes a World of format holding n objects of type search/a
// and returns its testbed and root.
func seedSearchWorld(t *testing.T, ctx context.Context, format world_block.WorldFormat, n int) (*testbed.Testbed, *bucket.ObjectRef) {
	t.Helper()

	// Write an empty World root of the format.
	le := logrus.NewEntry(logrus.New())
	tbed, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tbed.Release)
	ocs, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ocs.Release)
	btx, bcs := ocs.BuildTransaction(nil)
	bcs.SetBlock(world_block.NewWorld(format, false), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	ocs.SetRootRef(rootRef)

	// Write the objects in transactions of 1000.
	eng, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	for start := 0; start < n; start += 1000 {
		tx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := start; i < min(start+1000, n); i++ {
			obj, err := tx.CreateObject(ctx, searchObjectKey(i), nil)
			world.ReleaseObjectState(obj)
			if err == nil {
				err = world_types.SetObjectType(ctx, tx, searchObjectKey(i), "search/a")
			}
			if err != nil {
				tx.Discard()
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return tbed, eng.GetRootRef()
}

// searchObjectKey returns the key of search object i.
func searchObjectKey(i int) string {
	return "search/object/" + strconv.Itoa(i)
}
