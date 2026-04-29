package publish

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	spacewave_provider "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/hash"
)

func TestBuildPublishPlanNoOpWhenDestinationMatches(t *testing.T) {
	srcHeadRef := testPublishHeadRef("src-same")
	dstHeadRef := testPublishHeadRef("src-same")
	plan := BuildPublishPlan(
		[]*packfile.PackfileEntry{{Id: "01PACKA"}, {Id: "01PACKB"}},
		[]*packfile.PackfileEntry{{Id: "01PACKA"}, {Id: "01PACKB"}},
		srcHeadRef,
		dstHeadRef,
	)
	if len(plan.MissingPackIDs) != 0 {
		t.Fatalf("missing packs = %v", plan.MissingPackIDs)
	}
	if plan.NeedRootPost {
		t.Fatal("expected no-op root plan when destination head matches source")
	}
}

func TestBuildPublishPlanPartialRetrySkipsExistingPacks(t *testing.T) {
	srcHeadRef := testPublishHeadRef("src-new")
	dstHeadRef := testPublishHeadRef("dst-old")
	plan := BuildPublishPlan(
		[]*packfile.PackfileEntry{{Id: "01PACKA"}, {Id: "01PACKB"}},
		[]*packfile.PackfileEntry{{Id: "01PACKA"}},
		srcHeadRef,
		dstHeadRef,
	)
	if len(plan.MissingPackIDs) != 1 || plan.MissingPackIDs[0] != "01PACKB" {
		t.Fatalf("unexpected missing packs: %v", plan.MissingPackIDs)
	}
	if !plan.NeedRootPost {
		t.Fatal("expected root repost when destination head differs from source")
	}
}

func TestBuildPublishPlanRepairsMissingPacksWithoutRootPost(t *testing.T) {
	srcHeadRef := testPublishHeadRef("src-same")
	dstHeadRef := testPublishHeadRef("src-same")
	plan := BuildPublishPlan(
		[]*packfile.PackfileEntry{{Id: "01PACKA"}, {Id: "01PACKB"}},
		[]*packfile.PackfileEntry{{Id: "01PACKA"}},
		srcHeadRef,
		dstHeadRef,
	)
	if len(plan.MissingPackIDs) != 1 || plan.MissingPackIDs[0] != "01PACKB" {
		t.Fatalf("unexpected missing packs: %v", plan.MissingPackIDs)
	}
	if plan.NeedRootPost {
		t.Fatal("expected root post skip when destination head already matches source")
	}
}

func TestPromoteNoOpWhenDestinationMatches(t *testing.T) {
	const srcSpaceID = "01SRCSPACE000000000000000000"
	const dstSpaceID = "01DSTSPACE000000000000000000"
	headRef := testPublishObjectRef(1)
	client := &promoteTestClient{
		state: testPublishStateBytes(t, headRef),
		pulls: map[string][]byte{
			srcSpaceID: testPublishPullBytes(t, []*packfile.PackfileEntry{{Id: "01PACKA"}}),
			dstSpaceID: testPublishPullBytes(t, []*packfile.PackfileEntry{{Id: "01PACKA"}}),
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+dstSpaceID+"/root.packedmsg" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(testPublishRootPointer(t, dstSpaceID, headRef)))
	}))
	defer srv.Close()

	var out strings.Builder
	err := Promote(context.Background(), Options{
		Client:     client,
		Output:     &out,
		CdnBaseURL: srv.URL,
		SrcSpaceID: srcSpaceID,
		DstSpaceID: dstSpaceID,
	})
	if err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	if out.String() != "publish-space: no changes (destination already matches source)\n" {
		t.Fatalf("output = %q", out.String())
	}
	if client.pushes != 0 {
		t.Fatalf("pushes = %d", client.pushes)
	}
	if client.roots != 0 {
		t.Fatalf("roots = %d", client.roots)
	}
}

func testPublishHeadRef(id string) *bucket.ObjectRef {
	return &bucket.ObjectRef{
		BucketId: id,
	}
}

func testPublishObjectRef(seed byte) *bucket.ObjectRef {
	return &bucket.ObjectRef{
		RootRef: &block.BlockRef{
			Hash: &hash.Hash{
				HashType: hash.HashType_HashType_SHA256,
				Hash:     testPublishDigest(seed),
			},
		},
	}
}

func testPublishStateBytes(t *testing.T, headRef *bucket.ObjectRef) []byte {
	t.Helper()
	stateData, err := (&sobject_world_engine.InnerState{HeadRef: headRef}).MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT() error = %v", err)
	}
	inner, err := (&sobject.SORootInner{StateData: stateData}).MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT() error = %v", err)
	}
	out, err := (&api.SOStateMessage{
		Content: &api.SOStateMessage_Snapshot{
			Snapshot: &sobject.SOState{Root: &sobject.SORoot{Inner: inner}},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT() error = %v", err)
	}
	return out
}

func testPublishPullBytes(t *testing.T, entries []*packfile.PackfileEntry) []byte {
	t.Helper()
	out, err := (&packfile.PullResponse{Entries: entries}).MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT() error = %v", err)
	}
	return out
}

func testPublishRootPointer(t *testing.T, spaceID string, headRef *bucket.ObjectRef) string {
	t.Helper()
	stateData, err := (&sobject_world_engine.InnerState{HeadRef: headRef}).MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT() error = %v", err)
	}
	inner, err := (&sobject.SORootInner{StateData: stateData}).MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT() error = %v", err)
	}
	ptrBytes, err := (&alpha_cdn.CdnRootPointer{
		SpaceId: spaceID,
		Root:    &sobject.SORoot{Inner: inner},
	}).MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT() error = %v", err)
	}
	return packedmsg.EncodePackedMessage(ptrBytes)
}

func testPublishDigest(seed byte) []byte {
	out := make([]byte, 32)
	out[0] = seed
	return out
}

type promoteTestClient struct {
	state  []byte
	pulls  map[string][]byte
	pushes int
	roots  int
}

func (c *promoteTestClient) Do(*http.Request) (*http.Response, error) {
	return nil, nil
}

func (c *promoteTestClient) GetSOState(context.Context, string, uint64, spacewave_provider.SeedReason) ([]byte, error) {
	return c.state, nil
}

func (c *promoteTestClient) SyncPull(_ context.Context, resourceID string, _ string) ([]byte, error) {
	return c.pulls[resourceID], nil
}

func (c *promoteTestClient) SyncPushData(context.Context, string, string, int, []byte, []byte, []byte, uint32) error {
	c.pushes++
	return nil
}

func (c *promoteTestClient) PostRoot(context.Context, string, *sobject.SORoot, []*sobject.SOOperationRejection) error {
	c.roots++
	return nil
}
