package publish

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	spacewave_provider "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
)

// TestBuildPublishPlanNoOpWhenDestinationMatches checks that a matching
// destination needs no packs and no checkpoint.
func TestBuildPublishPlanNoOpWhenDestinationMatches(t *testing.T) {
	// Plan against a destination with the same packs and head.
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
	if plan.NeedCheckpoint {
		t.Fatal("expected no-op root plan when destination head matches source")
	}
}

// TestBuildPublishPlanCopiesAllSourcePacksWhenRootDiffers checks that a new
// head copies every source pack and posts a checkpoint.
func TestBuildPublishPlanCopiesAllSourcePacksWhenRootDiffers(t *testing.T) {
	// Plan against a destination with an older head.
	srcHeadRef := testPublishHeadRef("src-new")
	dstHeadRef := testPublishHeadRef("dst-old")
	plan := BuildPublishPlan(
		[]*packfile.PackfileEntry{{Id: "01PACKA"}, {Id: "01PACKB"}},
		[]*packfile.PackfileEntry{{Id: "01PACKA"}},
		srcHeadRef,
		dstHeadRef,
	)
	if len(plan.MissingPackIDs) != 2 ||
		plan.MissingPackIDs[0] != "01PACKA" ||
		plan.MissingPackIDs[1] != "01PACKB" {
		t.Fatalf("unexpected missing packs: %v", plan.MissingPackIDs)
	}
	if !plan.NeedCheckpoint {
		t.Fatal("expected root repost when destination head differs from source")
	}
}

// TestBuildPublishPlanSkipsPackRepairWhenRootAlreadyMatches checks that a
// matching head needs nothing even when the destination lacks packs.
func TestBuildPublishPlanSkipsPackRepairWhenRootAlreadyMatches(t *testing.T) {
	// Plan against a destination with the same head but fewer packs.
	srcHeadRef := testPublishHeadRef("src-same")
	dstHeadRef := testPublishHeadRef("src-same")
	plan := BuildPublishPlan(
		[]*packfile.PackfileEntry{{Id: "01PACKA"}, {Id: "01PACKB"}},
		[]*packfile.PackfileEntry{{Id: "01PACKA"}},
		srcHeadRef,
		dstHeadRef,
	)
	if len(plan.MissingPackIDs) != 0 {
		t.Fatalf("unexpected missing packs: %v", plan.MissingPackIDs)
	}
	if plan.NeedCheckpoint {
		t.Fatal("expected root post skip when destination head already matches source")
	}
}

// TestPromoteNoOpWhenDestinationMatches verifies Promote pushes nothing when
// the destination already holds the source packs and head.
func TestPromoteNoOpWhenDestinationMatches(t *testing.T) {
	// Give both spaces the same pack and head.
	const srcSpaceID = "01SRCSPACE000000000000000000"
	const dstSpaceID = "01DSTSPACE000000000000000000"
	headRef := testPublishObjectRef(1)
	client := &promoteTestClient{
		state: testPublishStateBytes(t, srcSpaceID, headRef),
		pulls: map[string]*packfile.PullResponse{
			srcSpaceID: {Entries: []*packfile.PackfileEntry{{Id: "01PACKA"}}},
			dstSpaceID: {Entries: []*packfile.PackfileEntry{{Id: "01PACKA"}}},
		},
	}

	// Serve the destination root pointer from the CDN.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+dstSpaceID+"/root.packedmsg" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(testPublishRootPointer(t, dstSpaceID, headRef)))
	}))
	defer srv.Close()

	// Promote reports no changes.
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

	// Nothing was pushed and no checkpoint was posted.
	if client.pushes != 0 {
		t.Fatalf("pushes = %d", client.pushes)
	}
	if client.checkpoints != 0 {
		t.Fatalf("checkpoints = %d", client.checkpoints)
	}
}

// TestFetchDestinationHeadRefRejectsMalformedCheckpoint checks that a root
// pointer with a malformed checkpoint body fails.
func TestFetchDestinationHeadRefRejectsMalformedCheckpoint(t *testing.T) {
	// Serve a root pointer whose checkpoint body is not a checkpoint.
	const dstSpaceID = "01DSTSPACE000000000000000000"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+dstSpaceID+"/root.packedmsg" {
			http.NotFound(w, r)
			return
		}
		ptrBytes, err := (&alpha_cdn.CdnRootPointer{
			SpaceId:    dstSpaceID,
			Checkpoint: &sobject.SOCheckpoint{Inner: []byte("not-a-checkpoint")},
		}).MarshalVT()
		if err != nil {
			t.Fatalf("MarshalVT() error = %v", err)
		}
		_, _ = w.Write([]byte(packedmsg.EncodePackedMessage(ptrBytes)))
	}))
	defer srv.Close()

	// Fetching the head fails to decode the checkpoint.
	_, err := FetchDestinationHeadRef(context.Background(), srv.URL, dstSpaceID)
	if err == nil {
		t.Fatal("expected malformed destination checkpoint error")
	}
	if !strings.Contains(err.Error(), "unmarshal checkpoint inner") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestFetchDestinationHeadRefRejectsMalformedInnerState checks that a
// checkpoint without World state fails.
func TestFetchDestinationHeadRefRejectsMalformedInnerState(t *testing.T) {
	// Serve a root pointer whose checkpoint state is not a World state.
	const dstSpaceID = "01DSTSPACE000000000000000000"
	checkpoint := testPublishCheckpoint(t, dstSpaceID, []byte("not-inner-state"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+dstSpaceID+"/root.packedmsg" {
			http.NotFound(w, r)
			return
		}
		ptrBytes, err := (&alpha_cdn.CdnRootPointer{
			SpaceId:    dstSpaceID,
			Checkpoint: checkpoint,
		}).MarshalVT()
		if err != nil {
			t.Fatalf("MarshalVT() error = %v", err)
		}
		_, _ = w.Write([]byte(packedmsg.EncodePackedMessage(ptrBytes)))
	}))
	defer srv.Close()

	// Fetching the head fails to decode the state.
	_, err := FetchDestinationHeadRef(context.Background(), srv.URL, dstSpaceID)
	if err == nil {
		t.Fatal("expected malformed destination inner state error")
	}
	if !strings.Contains(err.Error(), "unmarshal inner state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// testPublishHeadRef returns a head ref named by id.
func testPublishHeadRef(id string) *bucket.ObjectRef {
	return &bucket.ObjectRef{
		BucketId: id,
	}
}

// testPublishObjectRef returns an object ref whose root block hash derives from
// seed.
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

// testPublishCheckpoint signs a genesis checkpoint of spaceID holding
// stateData with disposable test material.
func testPublishCheckpoint(t *testing.T, spaceID string, stateData []byte) *sobject.SOCheckpoint {
	// Sign with a disposable key.
	t.Helper()
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := sobject.BuildGenesisSOCheckpoint(key, spaceID, make([]byte, 32), stateData)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

// testPublishHeadCheckpoint signs a genesis checkpoint of spaceID whose World
// is headRef.
func testPublishHeadCheckpoint(t *testing.T, spaceID string, headRef *bucket.ObjectRef) *sobject.SOCheckpoint {
	t.Helper()
	stateData, err := (&sobject_world_engine.InnerState{HeadRef: headRef}).MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT() error = %v", err)
	}
	return testPublishCheckpoint(t, spaceID, stateData)
}

func testPublishStateBytes(t *testing.T, spaceID string, headRef *bucket.ObjectRef) []byte {
	t.Helper()
	out, err := (&api.SOStateMessage{
		Content: &api.SOStateMessage_Snapshot{
			Snapshot: &sobject.SOState{Checkpoint: testPublishHeadCheckpoint(t, spaceID, headRef)},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT() error = %v", err)
	}
	return out
}

func testPublishRootPointer(t *testing.T, spaceID string, headRef *bucket.ObjectRef) string {
	t.Helper()
	ptrBytes, err := (&alpha_cdn.CdnRootPointer{
		SpaceId:    spaceID,
		Checkpoint: testPublishHeadCheckpoint(t, spaceID, headRef),
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
	state       []byte
	pulls       map[string]*packfile.PullResponse
	pushes      int
	checkpoints int
}

func (c *promoteTestClient) ReadGrants(context.Context, string, []string) ([]*packfile.ReadGrant, error) {
	return nil, nil
}

func (c *promoteTestClient) OpenPackReader(string, string, int64) (*packfile_store.PackReader, error) {
	return nil, nil
}

func (c *promoteTestClient) GetSOState(context.Context, string, uint64, spacewave_provider.SeedReason) ([]byte, error) {
	return c.state, nil
}

func (c *promoteTestClient) SyncPull(_ context.Context, resourceID string, _ uint64) (*packfile.PullResponse, error) {
	return c.pulls[resourceID], nil
}

func (c *promoteTestClient) SyncPushData(context.Context, string, string, int, []byte, []byte, []byte, uint32) error {
	c.pushes++
	return nil
}

func (c *promoteTestClient) PostCheckpoint(context.Context, string, *sobject.SOCheckpoint) error {
	c.checkpoints++
	return nil
}
