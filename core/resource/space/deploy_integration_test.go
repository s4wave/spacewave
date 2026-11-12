package resource_space

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	devtool_store "github.com/s4wave/spacewave/bldr/devtool/store"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/hash"
	deploy "github.com/s4wave/spacewave/sdk/deploy"
	"github.com/sirupsen/logrus"
)

type deployIntegrationBody struct {
	engine   world.Engine
	bucketID string
	engineID string
}

func (b *deployIntegrationBody) GetWorldEngine() world.Engine { return b.engine }
func (b *deployIntegrationBody) GetWorldEngineID() string {
	if b.engineID != "" {
		return b.engineID
	}
	return "test-world"
}
func (b *deployIntegrationBody) GetWorldEngineBucketID() string               { return b.bucketID }
func (b *deployIntegrationBody) GetSharedObjectRef() *sobject.SharedObjectRef { return nil }
func (b *deployIntegrationBody) GetSharedObject() sobject.SharedObject        { return nil }

type deployIntegrationStream struct {
	ctx              context.Context
	mu               sync.Mutex
	recv             []*deploy.DeployManifestsMessage
	blocks           map[string][]byte
	source           block.StoreOps
	sent             []*deploy.DeployManifestsMessage
	onBlockRequest   func(*block.BlockRef)
	wrongResponseRef bool
	wrongData        bool
}

func (s *deployIntegrationStream) Context() context.Context   { return s.ctx }
func (s *deployIntegrationStream) MsgSend(srpc.Message) error { return nil }
func (s *deployIntegrationStream) MsgRecv(srpc.Message) error { return errors.New("unused") }
func (s *deployIntegrationStream) CloseSend() error           { return nil }
func (s *deployIntegrationStream) Close() error               { return nil }
func (s *deployIntegrationStream) Send(m *deploy.DeployManifestsMessage) error {
	// Record deployment messages and supply requested manifest block responses.
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, m)
	if req := m.GetBlockRequest(); req != nil {
		if s.onBlockRequest != nil {
			s.onBlockRequest(req.GetRef())
		}
		data, ok := s.blocks[req.GetRef().MarshalString()]
		if s.source != nil {
			var err error
			data, ok, err = s.source.GetBlock(s.ctx, req.GetRef())
			if err != nil {
				return err
			}
		}
		responseRef := req.GetRef()
		if s.wrongResponseRef {
			responseRef = block.NewBlockRef(hash.NewHash(hash.HashType_HashType_SHA256, []byte("wrong-response")))
		}
		resp := &deploy.BlockResponse{Ref: responseRef, NotFound: !ok}
		if ok {
			resp.Data = data
			if s.wrongData {
				resp.Data = []byte("wrong-data")
			}
		}
		s.recv = append(s.recv, &deploy.DeployManifestsMessage{Body: &deploy.DeployManifestsMessage_BlockResponse{BlockResponse: resp}})
	}
	return nil
}

func (s *deployIntegrationStream) SendAndClose(m *deploy.DeployManifestsMessage) error {
	s.sent = append(s.sent, m)
	return nil
}

func (s *deployIntegrationStream) Recv() (*deploy.DeployManifestsMessage, error) {
	// Receive the next queued deployment message while the stream is active.
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.recv) == 0 {
		return nil, errors.New("no queued message")
	}
	m := s.recv[0]
	s.recv = s.recv[1:]
	return m, nil
}

func (s *deployIntegrationStream) RecvTo(*deploy.DeployManifestsMessage) error {
	return errors.New("unused")
}

func newDeployIntegrationEngine(t *testing.T, ctx context.Context) (*testbed.Testbed, *world_block.Engine) {
	// Start the block testbed and its manifest-aware World engine.
	t.Helper()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}
	eng, err := world_block.NewEngine(ctx, tb.Logger, cursor, world.NewSchema(bldr_manifest_world.WorldTypes).LookupOp, nil, false)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}
	return tb, eng
}

func integrationRef(t *testing.T, id, platform string, rev uint64, entry string) (*bldr_manifest.ManifestRef, []byte) {
	// Encode a manifest and calculate its deployment block reference.
	t.Helper()
	meta := &bldr_manifest.ManifestMeta{ManifestId: id, BuildType: "production", PlatformId: platform, Rev: rev}
	data, err := bldr_manifest.NewManifest(meta, entry).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	root, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return bldr_manifest.NewManifestRef(meta, &bucket.ObjectRef{RootRef: root}), data
}

func integrationRequest(refs ...*bldr_manifest.ManifestRef) *deploy.DeployManifestsMessage {
	return &deploy.DeployManifestsMessage{Body: &deploy.DeployManifestsMessage_Request{Request: &deploy.DeployManifestsRequest{ObjectKey: "plugin-host", ManifestRefs: refs}}}
}

func runIntegrationDeploy(t *testing.T, r *SpaceResource, ctx context.Context, blocks map[string][]byte, req *deploy.DeployManifestsMessage, onBlock func(*block.BlockRef)) error {
	// Queue the request on a stream that answers from blocks.
	s := &deployIntegrationStream{ctx: ctx, blocks: blocks, recv: []*deploy.DeployManifestsMessage{req}, onBlockRequest: onBlock}
	return runIntegrationStream(t, r, s)
}

func runIntegrationStream(t *testing.T, r *SpaceResource, s *deployIntegrationStream) error {
	// Run the Space manifest deployment and validate its result message.
	err := r.DeployManifests(s)
	if err != nil {
		return err
	}
	if len(s.sent) == 0 {
		t.Fatal("deployment sent no result")
	}
	if got := s.sent[len(s.sent)-1].GetResult(); got == nil {
		t.Fatal("deployment sent no result message")
	} else if got.GetError() != "" {
		return fmt.Errorf("remote result: %s", got.GetError())
	}
	return nil
}

func TestDeployManifestsPublishesNativeAndJSAndReplays(t *testing.T) {
	// Start a manifest-aware World engine for deployment replay.
	ctx := context.Background()
	tb, eng := newDeployIntegrationEngine(t, ctx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}

	// Publish a historical manifest and its host edge before deploying replacements.
	history, historyData := integrationRef(t, "gizmo-core", "js", 1, "history")
	tx, _ := eng.NewTransaction(ctx, true)
	if _, e := bldr_manifest_world.CreateManifestStore(ctx, tx, "plugin-host"); e != nil {
		t.Fatal(e)
	}
	hk := bldr_manifest.NewManifestKey("plugin-host", history.GetMeta())
	if _, _, e := bldr_manifest_world.SetManifest(ctx, tx, "", hk, &bucket.ObjectRef{BucketId: tb.BucketId, RootRef: history.GetManifestRef().GetRootRef()}); e != nil {
		t.Fatal(e)
	}
	if e := tx.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad("plugin-host", hk, "gizmo-core")); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	_, _ = eng.Sync(ctx)

	// Deploy the native and JavaScript manifest set through the Space stream.
	native, nativeData := integrationRef(t, "gizmo-core", "desktop/darwin/arm64", 2, "native")
	js, jsData := integrationRef(t, "gizmo-core", "js", 3, "js")
	blocks := map[string][]byte{native.GetManifestRef().GetRootRef().MarshalString(): nativeData, js.GetManifestRef().GetRootRef().MarshalString(): jsData, history.GetManifestRef().GetRootRef().MarshalString(): historyData}
	req := integrationRequest(native, js)
	if e := runIntegrationDeploy(t, r, ctx, blocks, req, nil); e != nil {
		t.Fatal(e)
	}

	// Verify both deployed manifest artifacts are present in the World.
	ws := world.NewEngineWorldState(eng, false)
	for _, ref := range []*bldr_manifest.ManifestRef{native, js} {
		key := bldr_manifest.NewManifestArtifactKey(ref.GetManifestRef())
		{
			objectState, ok, e := ws.GetObject(ctx, key)
			world.ReleaseObjectState(objectState)
			if e != nil || !ok {
				t.Fatalf("child %s missing: %v", key, e)
			}
		}
	}

	// Verify the host keeps its historical edge alongside both new manifests.
	edges, e := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("plugin-host", bldr_manifest_world.PredManifest.String(), "", ""), 0)
	if e != nil {
		t.Fatal(e)
	}
	if len(edges) != 3 {
		t.Fatalf("edge count=%d want 3 including history", len(edges))
	}

	// Replay the manifest deployment and verify it creates no extra host edges.
	if e := runIntegrationDeploy(t, r, ctx, blocks, req, nil); e != nil {
		t.Fatal(e)
	}
	edges2, e := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("plugin-host", bldr_manifest_world.PredManifest.String(), "", ""), 0)
	if e != nil {
		t.Fatal(e)
	}
	if len(edges2) != len(edges) {
		t.Fatalf("replay edge count=%d want %d", len(edges2), len(edges))
	}

	// Import the plugin into a second Space with its own block store: only the
	// newest build per platform arrives, and a repeat import changes nothing.
	tb2, eng2 := newDeployIntegrationEngine(t, ctx)
	defer tb2.Release()
	defer eng2.Close()
	r2 := &SpaceResource{le: tb2.Logger, space: &deployIntegrationBody{engine: eng2, bucketID: tb2.BucketId, engineID: "test-world-2"}}
	if e := r2.importPlugin(ctx, r2, "gizmo-core"); e == nil {
		t.Fatal("import from the same Space succeeded")
	}
	if e := r2.importPlugin(ctx, r, "missing-plugin"); e == nil {
		t.Fatal("import of a missing plugin succeeded")
	}
	for range 2 {
		if e := r2.importPlugin(ctx, r, "gizmo-core"); e != nil {
			t.Fatal(e)
		}
		ws2 := world.NewEngineWorldState(eng2, false)
		imported, _, e := bldr_manifest_world.CollectManifestsForManifestID(ctx, ws2, "gizmo-core", nil, "plugin-host")
		if e != nil {
			t.Fatal(e)
		}
		var revs []uint64
		for _, m := range imported {
			revs = append(revs, m.GetRev())
		}
		slices.Sort(revs)
		if !slices.Equal(revs, []uint64{2, 3}) {
			t.Fatalf("imported revs=%v want [2 3]", revs)
		}
	}
}

func TestDeployManifestsMissingBlockAndWrongHostDoNotPublish(t *testing.T) {
	// Start a manifest-aware World engine for rejected deployments.
	ctx := context.Background()
	tb, eng := newDeployIntegrationEngine(t, ctx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}

	// Attempt a manifest set whose JavaScript block is missing.
	native, nativeData := integrationRef(t, "gizmo-core", "desktop/darwin/arm64", 2, "native")
	js, _ := integrationRef(t, "gizmo-core", "js", 3, "js")
	blocks := map[string][]byte{native.GetManifestRef().GetRootRef().MarshalString(): nativeData}
	if e := runIntegrationDeploy(t, r, ctx, blocks, integrationRequest(native, js), nil); e == nil {
		t.Fatal("missing block accepted")
	}

	// Verify the missing block prevents manifest edge publication.
	ws := world.NewEngineWorldState(eng, false)
	edges, _ := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("plugin-host", bldr_manifest_world.PredManifest.String(), "", ""), 0)
	if len(edges) != 0 {
		t.Fatalf("missing block published %d edges", len(edges))
	}

	// Create a World object with a type that cannot host manifests.
	tx, _ := eng.NewTransaction(ctx, true)
	{
		createdObject, e := tx.CreateObject(ctx, "wrong-host", nil)
		world.ReleaseObjectState(createdObject)
		if e != nil {
			t.Fatal(e)
		}
	}
	if e := world_types.SetObjectType(ctx, tx, "wrong-host", "other-type"); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	_, _ = eng.Sync(ctx)

	// Verify deployment rejects the object with the wrong host type.
	if e := runIntegrationDeploy(t, r, ctx, map[string][]byte{native.GetManifestRef().GetRootRef().MarshalString(): nativeData}, &deploy.DeployManifestsMessage{Body: &deploy.DeployManifestsMessage_Request{Request: &deploy.DeployManifestsRequest{ObjectKey: "wrong-host", ManifestRefs: []*bldr_manifest.ManifestRef{native}}}}, nil); e == nil {
		t.Fatal("wrong host accepted")
	}
}

// TestDeployManifestsRejectsMaskedRevision rejects a build the Space would never
// start because it already holds a newer build for the same platform.
func TestDeployManifestsRejectsMaskedRevision(t *testing.T) {
	// Start the World engine used to test masked manifest revisions.
	ctx := context.Background()
	tb, eng := newDeployIntegrationEngine(t, ctx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}

	// Deploy a newer build, then an older one for the same platform.
	newer, newerData := integrationRef(t, "gizmo-core", "desktop/darwin/arm64", 42, "newer")
	older, olderData := integrationRef(t, "gizmo-core", "desktop/darwin/arm64", 36, "older")
	blocks := map[string][]byte{
		newer.GetManifestRef().GetRootRef().MarshalString(): newerData,
		older.GetManifestRef().GetRootRef().MarshalString(): olderData,
	}
	if e := runIntegrationDeploy(t, r, ctx, blocks, integrationRequest(newer), nil); e != nil {
		t.Fatal(e)
	}
	e := runIntegrationDeploy(t, r, ctx, blocks, integrationRequest(older), nil)
	if e == nil || !strings.Contains(e.Error(), "rev 36 would not run") {
		t.Fatalf("masked deploy error = %v", e)
	}

	// The rejected build stays unpublished.
	ws := world.NewEngineWorldState(eng, false)
	edges, e := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("plugin-host", bldr_manifest_world.PredManifest.String(), "", ""), 0)
	if e != nil {
		t.Fatal(e)
	}
	if len(edges) != 1 {
		t.Fatalf("edge count=%d want 1", len(edges))
	}
}

// TestDeployManifestsRepinsPinnedInstallation checks that a deploy publishes
// its artifact as the Space's installation when the settings already pin the
// plugin, with the earlier pin retained for rollback.
func TestDeployManifestsRepinsPinnedInstallation(t *testing.T) {
	// Start a manifest-aware World engine for deployment repinning.
	ctx := context.Background()
	tb, eng := newDeployIntegrationEngine(t, ctx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}

	// Deploy the first build of the plugin to the Space.
	older, olderData := integrationRef(t, "gizmo-core", "desktop/darwin/arm64", 1, "older")
	if e := runIntegrationDeploy(t, r, ctx, map[string][]byte{older.GetManifestRef().GetRootRef().MarshalString(): olderData}, integrationRequest(older), nil); e != nil {
		t.Fatal(e)
	}

	// Wait for the deployed manifest to publish, then pin it like a Forge
	// build does.
	if _, e := eng.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	olderKey := bldr_manifest.NewManifestArtifactKey(older.GetManifestRef())
	tx, e := eng.NewTransaction(ctx, true)
	if e != nil {
		t.Fatal(e)
	}
	if changed, e := space_world_ops.InstallSpacePlugin(ctx, tx, "", "gizmo-core", olderKey, time.Now()); e != nil || !changed {
		tx.Discard()
		t.Fatalf("initial pin: changed=%v err=%v", changed, e)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	tx.Discard()

	// Deploy the higher revision for the same platform.
	newer, newerData := integrationRef(t, "gizmo-core", "desktop/darwin/arm64", 2, "newer")
	if e := runIntegrationDeploy(t, r, ctx, map[string][]byte{newer.GetManifestRef().GetRootRef().MarshalString(): newerData}, integrationRequest(newer), nil); e != nil {
		t.Fatal(e)
	}

	// The deployed artifact leads the installation and the earlier pin
	// remains as rollback.
	ws := world.NewEngineWorldState(eng, false)
	settings, e := space_world.LookupSpaceSettingsBody(ctx, ws)
	if e != nil {
		t.Fatal(e)
	}
	keys := settings.GetPluginInstallations()["gizmo-core"].GetManifestKeys()
	newerKey := bldr_manifest.NewManifestArtifactKey(newer.GetManifestRef())
	if !slices.Equal(keys, []string{newerKey, olderKey}) {
		t.Fatalf("installed artifacts %v, want [%s %s]", keys, newerKey, olderKey)
	}
}

// TestDeployManifestsLeavesUnpinnedSpaceAlone checks that a deploy into a
// Space with no pin keeps creating no installation entry.
func TestDeployManifestsLeavesUnpinnedSpaceAlone(t *testing.T) {
	// Start a manifest-aware World engine for an unpinned deployment.
	ctx := context.Background()
	tb, eng := newDeployIntegrationEngine(t, ctx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}

	// Deploy one build into the Space.
	ref, data := integrationRef(t, "gizmo-core", "js", 1, "js")
	if e := runIntegrationDeploy(t, r, ctx, map[string][]byte{ref.GetManifestRef().GetRootRef().MarshalString(): data}, integrationRequest(ref), nil); e != nil {
		t.Fatal(e)
	}

	// Verify the deployment published the manifest and no installation.
	ws := world.NewEngineWorldState(eng, false)
	settings, e := space_world.LookupSpaceSettingsBody(ctx, ws)
	if e != nil {
		t.Fatal(e)
	}
	if settings.GetPluginInstallations()["gizmo-core"].GetManifestKeys() != nil {
		t.Fatalf("unpinned deploy created an installation: %v", settings.GetPluginInstallations())
	}
}

func TestDeployManifestsCancellationAndExactBlockExchangeRejectPublication(t *testing.T) {
	// Start the World engine used to test invalid block exchanges.
	baseCtx := context.Background()
	tb, eng := newDeployIntegrationEngine(t, baseCtx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}

	// Prepare a manifest request and the invalid exchange cases.
	ref, data := integrationRef(t, "gizmo-core", "js", 1, "js")
	blocks := map[string][]byte{ref.GetManifestRef().GetRootRef().MarshalString(): data}
	req := integrationRequest(ref)
	cases := []struct {
		name                        string
		wrongRef, wrongData, cancel bool
	}{
		{name: "response ref", wrongRef: true},
		{name: "response data", wrongData: true},
		{name: "cancel during block send", cancel: true},
	}

	// Verify every invalid block exchange leaves the manifest set unpublished.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare the deployment stream with the selected exchange failure.
			ctx, cancel := context.WithCancel(baseCtx)
			defer cancel()
			s := &deployIntegrationStream{ctx: ctx, blocks: blocks, wrongResponseRef: tc.wrongRef, wrongData: tc.wrongData}
			if tc.cancel {
				s.onBlockRequest = func(*block.BlockRef) { cancel() }
			}
			s.recv = []*deploy.DeployManifestsMessage{req}

			// Run the invalid block exchange through the Space deployment service.
			if e := r.DeployManifests(s); e != nil {
				t.Fatal(e)
			}

			// Verify the exchange failure is reported without publishing manifest edges.
			if len(s.sent) == 0 || s.sent[len(s.sent)-1].GetResult().GetError() == "" {
				t.Fatal("invalid exchange succeeded")
			}
			ws := world.NewEngineWorldState(eng, false)
			edges, _ := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("plugin-host", bldr_manifest_world.PredManifest.String(), "", ""), 0)
			if len(edges) != 0 {
				t.Fatalf("invalid exchange published %d edges", len(edges))
			}
		})
	}
}

func TestDeployManifestsInTransactionFenceRejectsWrongHost(t *testing.T) {
	// Start the World engine used to test the transaction host check.
	ctx := context.Background()
	tb, eng := newDeployIntegrationEngine(t, ctx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}

	// Prepare a manifest and change its host type during block transfer.
	native, data := integrationRef(t, "gizmo-core", "desktop/darwin/arm64", 2, "native")
	blocks := map[string][]byte{native.GetManifestRef().GetRootRef().MarshalString(): data}
	onBlock := func(*block.BlockRef) {
		// Open a write transaction for the concurrent host type change.
		tx, e := eng.NewTransaction(ctx, true)
		if e != nil {
			return
		}

		// Create the host with a non-manifest type before deployment publication.
		var createdObject world.ObjectState
		createdObject, e = tx.CreateObject(ctx, "fenced-host", nil)
		world.ReleaseObjectState(createdObject)
		if e == nil {
			e = world_types.SetObjectType(ctx, tx, "fenced-host", "other-type")
		}

		// Publish the concurrent host type change and release its transaction.
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e == nil {
			_, _ = eng.Sync(ctx)
		}
		tx.Discard()
	}

	// Attempt deployment after the host type changes during transfer.
	req := &deploy.DeployManifestsMessage{Body: &deploy.DeployManifestsMessage_Request{Request: &deploy.DeployManifestsRequest{ObjectKey: "fenced-host", ManifestRefs: []*bldr_manifest.ManifestRef{native}}}}
	if e := runIntegrationDeploy(t, r, ctx, blocks, req, onBlock); e == nil {
		t.Fatal("in-transaction fence accepted")
	}

	// Verify the transaction host check prevents manifest edge publication.
	ws := world.NewEngineWorldState(eng, false)
	edges, _ := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("fenced-host", bldr_manifest_world.PredManifest.String(), "", ""), 0)
	if len(edges) != 0 {
		t.Fatalf("fenced host published %d edges", len(edges))
	}
}

// TestDeployManifestsFromExport deploys a build's export, as space deploy --db
// does, into a fresh Space: the Space holds the manifest at its rev and every
// block the manifest names, byte for byte. An export missing one block is
// refused whole and leaves the Space without the manifest.
func TestDeployManifestsFromExport(t *testing.T) {
	// Export a manifest with dist and assets trees.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	dir, built := writeExportedManifest(t, ctx, le)
	root := built.GetManifestRef().GetRootRef()

	// Open the export as space deploy --db does and list the blocks it names.
	exportVol, refs := openExport(t, ctx, le, dir, built)
	exported, exportedRefs := readManifestGraph(t, ctx, le, exportBucket(exportVol), root)
	if len(exported) < 5 {
		t.Fatalf("export names %d blocks, want the manifest and both trees", len(exported))
	}

	// Serve the export without one block below the manifest, as an export
	// that lost it would; its deploy fails and the Space holds no manifest.
	// The Volume frees blocks only through its collector, so the test hides
	// the block instead of removing it.
	damaged := &missingBlockStore{StoreOps: exportVol, missing: exportedRefs[len(exportedRefs)-1].MarshalString()}
	tb, eng := newDeployIntegrationEngine(t, ctx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}
	s := &deployIntegrationStream{ctx: ctx, source: damaged, recv: []*deploy.DeployManifestsMessage{integrationRequest(refs...)}}
	if err := runIntegrationStream(t, r, s); err == nil {
		t.Fatal("export missing a block was deployed")
	}
	ws := world.NewEngineWorldState(eng, false)
	if got, _, err := bldr_manifest_world.CollectManifestsForManifestID(ctx, ws, "gizmo-core", nil, "plugin-host"); err != nil || len(got) != 0 {
		t.Fatalf("export missing a block published %d manifests: %v", len(got), err)
	}

	// Deploy the intact export into a fresh Space.
	tb2, eng2 := newDeployIntegrationEngine(t, ctx)
	defer tb2.Release()
	defer eng2.Close()
	r2 := &SpaceResource{le: tb2.Logger, space: &deployIntegrationBody{engine: eng2, bucketID: tb2.BucketId}}
	s2 := &deployIntegrationStream{ctx: ctx, source: exportVol, recv: []*deploy.DeployManifestsMessage{integrationRequest(refs...)}}
	if err := runIntegrationStream(t, r2, s2); err != nil {
		t.Fatal(err)
	}

	// Read the manifest back at its rev, then every block it names.
	ws2 := world.NewEngineWorldState(eng2, false)
	got, _, err := bldr_manifest_world.CollectManifestsForManifestID(ctx, ws2, "gizmo-core", nil, "plugin-host")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Manifest.GetMeta().GetRev() != 7 || got[0].Manifest.GetEntrypoint() != "entry.mjs" {
		t.Fatalf("deployed manifests = %v", got)
	}
	err = ws2.AccessWorldState(ctx, got[0].ManifestRef, func(bls *bucket_lookup.Cursor) error {
		deployed, _ := readManifestGraph(t, ctx, le, bls.GetBucket(), root)
		if !maps.EqualFunc(deployed, exported, bytes.Equal) {
			return fmt.Errorf("Space holds %d blocks of the manifest, export names %d", len(deployed), len(exported))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// writeExportedManifest builds a manifest with dist and assets trees in a
// devtool World, as a build does, exports it with ExportManifestSets, and
// returns the export directory and the built ref.
func writeExportedManifest(t *testing.T, ctx context.Context, le *logrus.Entry) (string, *bldr_manifest.ManifestRef) {
	// Open the source devtool World and a stage that holds the new blocks.
	t.Helper()
	srcVol, err := devtool_store.OpenVolumeFile(ctx, le, filepath.Join(t.TempDir(), devtool_store.FileName), true)
	if err != nil {
		t.Fatal(err)
	}
	defer srcVol.Close()
	srcEng, err := devtool_store.OpenWorld(ctx, le, srcVol, true)
	if err != nil {
		t.Fatal(err)
	}
	defer srcEng.Close()
	stage, err := srcEng.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Release()

	// Write the manifest and its trees through the World's bucket.
	meta := &bldr_manifest.ManifestMeta{ManifestId: "gizmo-core", BuildType: "production", PlatformId: "js", Rev: 7}
	dist := fstest.MapFS{
		"entry.mjs":    {Data: []byte("export default 1\n")},
		"lib/util.mjs": {Data: []byte("export const x = 2\n")},
	}
	assets := fstest.MapFS{"index.html": {Data: []byte("<!doctype html>\n")}}
	var ref *bldr_manifest.ManifestRef
	err = stage.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		btx, bcs := bls.BuildTransactionAtRef(nil, nil)
		err := bldr_manifest.CreateManifestWithIoFS(ctx, bcs, bldr_manifest.NewManifest(meta, "entry.mjs"), dist, assets, unixfs_block.ToTimestamp(time.Unix(1, 0), false))
		if err != nil {
			return err
		}
		root, _, err := btx.Write(ctx, true)
		if err != nil {
			return err
		}
		objRef := bls.GetRef().Clone()
		objRef.RootRef = root
		ref = bldr_manifest.NewManifestRef(meta, objRef)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Export the manifest while the stage still holds its blocks.
	dir := t.TempDir()
	if err := devtool_store.ExportManifestSets(ctx, le, world.NewEngineWorldState(srcEng, true), []*bldr_manifest.ManifestRef{ref}, dir); err != nil {
		t.Fatal(err)
	}
	return dir, ref
}

// openExport opens the export in dir and looks up its manifest set, which
// must be the built ref alone.
func openExport(t *testing.T, ctx context.Context, le *logrus.Entry, dir string, built *bldr_manifest.ManifestRef) (volume.Volume, []*bldr_manifest.ManifestRef) {
	// Open the volume and close it when the test ends.
	t.Helper()
	vol, err := devtool_store.OpenVolume(ctx, le, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	refs, err := devtool_store.LookupManifestSet(ctx, le, vol, "gizmo-core")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || !refs[0].EqualVT(built) {
		t.Fatalf("exported set = %v", refs)
	}
	return vol, refs
}

// missingBlockStore serves a store without one block.
type missingBlockStore struct {
	block.StoreOps
	missing string
}

// GetBlock reports the missing block as absent.
func (s *missingBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if ref.MarshalString() == s.missing {
		return nil, false, nil
	}
	return s.StoreOps.GetBlock(ctx, ref)
}

// exportBucket returns the block store of an export's devtool bucket.
func exportBucket(vol volume.Volume) block.StoreOps {
	return volume_controller.NewBucketStore(vol, &bucket.Config{Id: devtool_store.BucketID, Rev: 1})
}

// readManifestGraph reads every block reachable from root in src by copying
// the graph into a scratch store. It returns each block's data keyed by ref,
// and the refs in the order the copy read them, root first.
func readManifestGraph(t *testing.T, ctx context.Context, le *logrus.Entry, src block.StoreOps, root *block.BlockRef) (map[string][]byte, []*block.BlockRef) {
	// Open the scratch store the copy writes into.
	t.Helper()
	scratch, err := devtool_store.OpenVolumeFile(ctx, le, filepath.Join(t.TempDir(), devtool_store.FileName), true)
	if err != nil {
		t.Fatal(err)
	}
	defer scratch.Close()

	// Record each block the copy reads.
	blocks := make(map[string][]byte)
	var refs []*block.BlockRef
	err = block.CopyGraph(ctx, src, exportBucket(scratch), root, &block.GraphCopyOptions{
		Visited: func(ref *block.BlockRef, data []byte) {
			blocks[ref.MarshalString()] = bytes.Clone(data)
			refs = append(refs, ref.Clone())
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return blocks, refs
}
