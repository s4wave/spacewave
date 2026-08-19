package resource_space

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
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
}

func (b *deployIntegrationBody) GetWorldEngine() world.Engine                 { return b.engine }
func (b *deployIntegrationBody) GetWorldEngineID() string                     { return "test-world" }
func (b *deployIntegrationBody) GetWorldEngineBucketID() string               { return b.bucketID }
func (b *deployIntegrationBody) GetSharedObjectRef() *sobject.SharedObjectRef { return nil }
func (b *deployIntegrationBody) GetSharedObject() sobject.SharedObject        { return nil }

type deployIntegrationStream struct {
	ctx              context.Context
	mu               sync.Mutex
	recv             []*deploy.DeployManifestsMessage
	blocks           map[string][]byte
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, m)
	if req := m.GetBlockRequest(); req != nil {
		if s.onBlockRequest != nil {
			s.onBlockRequest(req.GetRef())
		}
		data, ok := s.blocks[req.GetRef().MarshalString()]
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
	eng, err := world_block.NewEngine(ctx, tb.Logger, cursor, bldr_manifest_world.LookupOp, nil, false)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}
	return tb, eng
}

func integrationRef(t *testing.T, id, platform string, rev uint64, entry string) (*bldr_manifest.ManifestRef, []byte) {
	t.Helper()
	meta := &bldr_manifest.ManifestMeta{ManifestId: id, BuildType: "production", PlatformId: platform, Rev: rev}
	data, err := bldr_manifest.NewManifest(meta, entry).MarshalBlock()
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
	s := &deployIntegrationStream{ctx: ctx, blocks: blocks, recv: []*deploy.DeployManifestsMessage{req}, onBlockRequest: onBlock}
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
	ctx := context.Background()
	tb, eng := newDeployIntegrationEngine(t, ctx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}
	history, historyData := integrationRef(t, "glados-core", "js", 1, "history")
	tx, _ := eng.NewTransaction(ctx, true)
	if _, e := bldr_manifest_world.CreateManifestStore(ctx, tx, "plugin-host"); e != nil {
		t.Fatal(e)
	}
	hk := bldr_manifest.NewManifestKey("plugin-host", history.GetMeta())
	if _, _, e := bldr_manifest_world.SetManifest(ctx, tx, "", hk, &bucket.ObjectRef{BucketId: tb.BucketId, RootRef: history.GetManifestRef().GetRootRef()}); e != nil {
		t.Fatal(e)
	}
	if e := tx.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad("plugin-host", hk, "glados-core")); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	_, _ = eng.Sync(ctx)
	native, nativeData := integrationRef(t, "glados-core", "desktop/darwin/arm64", 2, "native")
	js, jsData := integrationRef(t, "glados-core", "js", 3, "js")
	blocks := map[string][]byte{native.GetManifestRef().GetRootRef().MarshalString(): nativeData, js.GetManifestRef().GetRootRef().MarshalString(): jsData, history.GetManifestRef().GetRootRef().MarshalString(): historyData}
	req := integrationRequest(native, js)
	if e := runIntegrationDeploy(t, r, ctx, blocks, req, nil); e != nil {
		t.Fatal(e)
	}
	ws := world.NewEngineWorldState(eng, false)
	for _, ref := range []*bldr_manifest.ManifestRef{native, js} {
		key := bldr_manifest.NewManifestKey("plugin-host", ref.GetMeta())
		if _, ok, e := ws.GetObject(ctx, key); e != nil || !ok {
			t.Fatalf("child %s missing: %v", key, e)
		}
	}
	edges, e := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("plugin-host", bldr_manifest_world.PredManifest.String(), "", ""), 0)
	if e != nil {
		t.Fatal(e)
	}
	if len(edges) != 3 {
		t.Fatalf("edge count=%d want 3 including history", len(edges))
	}
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
}

func TestDeployManifestsMissingBlockAndWrongHostDoNotPublish(t *testing.T) {
	ctx := context.Background()
	tb, eng := newDeployIntegrationEngine(t, ctx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}
	native, nativeData := integrationRef(t, "glados-core", "desktop/darwin/arm64", 2, "native")
	js, _ := integrationRef(t, "glados-core", "js", 3, "js")
	blocks := map[string][]byte{native.GetManifestRef().GetRootRef().MarshalString(): nativeData}
	if e := runIntegrationDeploy(t, r, ctx, blocks, integrationRequest(native, js), nil); e == nil {
		t.Fatal("missing block accepted")
	}
	ws := world.NewEngineWorldState(eng, false)
	edges, _ := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("plugin-host", bldr_manifest_world.PredManifest.String(), "", ""), 0)
	if len(edges) != 0 {
		t.Fatalf("missing block published %d edges", len(edges))
	}
	tx, _ := eng.NewTransaction(ctx, true)
	if _, e := tx.CreateObject(ctx, "wrong-host", nil); e != nil {
		t.Fatal(e)
	}
	if e := world_types.SetObjectType(ctx, tx, "wrong-host", "other-type"); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	_, _ = eng.Sync(ctx)
	if e := runIntegrationDeploy(t, r, ctx, map[string][]byte{native.GetManifestRef().GetRootRef().MarshalString(): nativeData}, &deploy.DeployManifestsMessage{Body: &deploy.DeployManifestsMessage_Request{Request: &deploy.DeployManifestsRequest{ObjectKey: "wrong-host", ManifestRefs: []*bldr_manifest.ManifestRef{native}}}}, nil); e == nil {
		t.Fatal("wrong host accepted")
	}
}

func TestDeployManifestsCancellationAndExactBlockExchangeRejectPublication(t *testing.T) {
	baseCtx := context.Background()
	tb, eng := newDeployIntegrationEngine(t, baseCtx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}
	ref, data := integrationRef(t, "glados-core", "js", 1, "js")
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
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(baseCtx)
			defer cancel()
			s := &deployIntegrationStream{ctx: ctx, blocks: blocks, wrongResponseRef: tc.wrongRef, wrongData: tc.wrongData}
			if tc.cancel {
				s.onBlockRequest = func(*block.BlockRef) { cancel() }
			}
			s.recv = []*deploy.DeployManifestsMessage{req}
			if e := r.DeployManifests(s); e != nil {
				t.Fatal(e)
			}
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
	ctx := context.Background()
	tb, eng := newDeployIntegrationEngine(t, ctx)
	defer tb.Release()
	defer eng.Close()
	r := &SpaceResource{le: tb.Logger, space: &deployIntegrationBody{engine: eng, bucketID: tb.BucketId}}
	native, data := integrationRef(t, "glados-core", "desktop/darwin/arm64", 2, "native")
	blocks := map[string][]byte{native.GetManifestRef().GetRootRef().MarshalString(): data}
	onBlock := func(*block.BlockRef) {
		tx, e := eng.NewTransaction(ctx, true)
		if e != nil {
			return
		}
		_, e = tx.CreateObject(ctx, "fenced-host", nil)
		if e == nil {
			e = world_types.SetObjectType(ctx, tx, "fenced-host", "other-type")
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e == nil {
			_, _ = eng.Sync(ctx)
		}
		tx.Discard()
	}
	req := &deploy.DeployManifestsMessage{Body: &deploy.DeployManifestsMessage_Request{Request: &deploy.DeployManifestsRequest{ObjectKey: "fenced-host", ManifestRefs: []*bldr_manifest.ManifestRef{native}}}}
	if e := runIntegrationDeploy(t, r, ctx, blocks, req, onBlock); e == nil {
		t.Fatal("in-transaction fence accepted")
	}
	ws := world.NewEngineWorldState(eng, false)
	edges, _ := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("fenced-host", bldr_manifest_world.PredManifest.String(), "", ""), 0)
	if len(edges) != 0 {
		t.Fatalf("fenced host published %d edges", len(edges))
	}
}
