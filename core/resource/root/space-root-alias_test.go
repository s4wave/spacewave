//go:build !js

package resource_root

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/core/session"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"github.com/sirupsen/logrus"
)

func TestSpaceRootAliasRegistryPersistsAndRemoves(t *testing.T) {
	ctx := t.Context()
	server, cancel := setupSpaceRootAliasServer(ctx, t)
	defer cancel()
	rootPath := makeSpaceRootAliasDir(t)

	record, err := server.UpsertSpaceRootAlias(ctx, &s4wave_root.UpsertSpaceRootAliasRequest{
		Record: &s4wave_root.SpaceRootAliasRecord{
			AliasId:     "company",
			DisplayName: "Company",
			Kind:        s4wave_root.SpaceRootKind_SpaceRootKind_NATIVE_DIRECTORY,
			OpenMode:    s4wave_root.SpaceRootOpenMode_SpaceRootOpenMode_OPEN_EXISTING,
			Native:      &s4wave_root.NativeSpaceRootMetadata{Path: rootPath},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.GetRecord().GetStatus() != s4wave_root.SpaceRootStatus_SpaceRootStatus_READY {
		t.Fatalf("status = %s, want ready", record.GetRecord().GetStatus())
	}

	records, err := server.snapshotSpaceRootAliases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].GetAliasId() != "company" || records[0].GetNative().GetPath() != rootPath {
		t.Fatalf("records = %#v, want company at %s", records, rootPath)
	}

	removeResp, err := server.RemoveSpaceRootAlias(ctx, &s4wave_root.RemoveSpaceRootAliasRequest{
		AliasId: "company",
	})
	if err != nil {
		t.Fatal(err)
	}
	if removeResp.GetNotFound() {
		t.Fatal("remove should find existing alias")
	}

	records, err = server.snapshotSpaceRootAliases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("records after remove = %#v, want empty", records)
	}
}

func TestSpaceRootAliasRegistryRejectsUnsupportedSelections(t *testing.T) {
	ctx := t.Context()
	server, cancel := setupSpaceRootAliasServer(ctx, t)
	defer cancel()

	if _, err := server.UpsertSpaceRootAlias(ctx, &s4wave_root.UpsertSpaceRootAliasRequest{
		Record: &s4wave_root.SpaceRootAliasRecord{
			AliasId:  "file",
			Kind:     s4wave_root.SpaceRootKind_SpaceRootKind_S4WAVE_FILE,
			OpenMode: s4wave_root.SpaceRootOpenMode_SpaceRootOpenMode_OPEN_EXISTING,
			Native:   &s4wave_root.NativeSpaceRootMetadata{Path: filepath.Join(t.TempDir(), "x.s4wave")},
		},
	}); err == nil {
		t.Fatal("expected .s4wave rejection")
	}

	plainDir := t.TempDir()
	if _, err := server.UpsertSpaceRootAlias(ctx, &s4wave_root.UpsertSpaceRootAliasRequest{
		Record: &s4wave_root.SpaceRootAliasRecord{
			AliasId:  "plain",
			Kind:     s4wave_root.SpaceRootKind_SpaceRootKind_NATIVE_DIRECTORY,
			OpenMode: s4wave_root.SpaceRootOpenMode_SpaceRootOpenMode_OPEN_EXISTING,
			Native:   &s4wave_root.NativeSpaceRootMetadata{Path: plainDir},
		},
	}); err == nil {
		t.Fatal("expected non-root directory rejection")
	}
}

func TestSpaceRootRuntimeReportsMissingDaemon(t *testing.T) {
	ctx := t.Context()
	server, cancel := setupSpaceRootAliasServer(ctx, t)
	defer cancel()
	rootPath := makeSpaceRootAliasDir(t)

	_, err := server.UpsertSpaceRootAlias(ctx, &s4wave_root.UpsertSpaceRootAliasRequest{
		Record: &s4wave_root.SpaceRootAliasRecord{
			AliasId:  "company",
			Kind:     s4wave_root.SpaceRootKind_SpaceRootKind_NATIVE_DIRECTORY,
			OpenMode: s4wave_root.SpaceRootOpenMode_SpaceRootOpenMode_OPEN_EXISTING,
			Native:   &s4wave_root.NativeSpaceRootMetadata{Path: rootPath},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	strm := &testSpaceRootRuntimeStream{ctx: ctx}
	if err := server.WatchSpaceRootRuntime(&s4wave_root.WatchSpaceRootRuntimeRequest{
		AliasId: "company",
	}, strm); err != nil {
		t.Fatal(err)
	}

	if len(strm.sent) != 2 {
		t.Fatalf("sent %d responses, want connecting and error", len(strm.sent))
	}
	if strm.sent[0].GetStatus() != s4wave_root.SpaceRootRuntimeStatus_SpaceRootRuntimeStatus_CONNECTING {
		t.Fatalf("first status = %s, want connecting", strm.sent[0].GetStatus())
	}
	if strm.sent[1].GetStatus() != s4wave_root.SpaceRootRuntimeStatus_SpaceRootRuntimeStatus_ERROR {
		t.Fatalf("second status = %s, want error", strm.sent[1].GetStatus())
	}
	if strm.sent[1].GetError() == "" {
		t.Fatal("expected actionable runtime error")
	}
}

func TestSpaceRootRuntimeStreamsSelectedRootSessions(t *testing.T) {
	watchCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server, serverCancel := setupSpaceRootAliasServer(t.Context(), t)
	defer serverCancel()
	rootPath := makeSpaceRootAliasDir(t)

	_, err := server.UpsertSpaceRootAlias(t.Context(), &s4wave_root.UpsertSpaceRootAliasRequest{
		Record: &s4wave_root.SpaceRootAliasRecord{
			AliasId:  "company",
			Kind:     s4wave_root.SpaceRootKind_SpaceRootKind_NATIVE_DIRECTORY,
			OpenMode: s4wave_root.SpaceRootOpenMode_SpaceRootOpenMode_OPEN_EXISTING,
			Native:   &s4wave_root.NativeSpaceRootMetadata{Path: rootPath},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	prev := connectSpaceRootRuntimeFunc
	connectSpaceRootRuntimeFunc = func(context.Context, string) (*spaceRootRuntimeClient, error) {
		return &spaceRootRuntimeClient{
			root: &testSpaceRootRuntimeRoot{
				sessions: []*session.SessionListEntry{{SessionIndex: 7}},
				metadata: map[uint32]*session.SessionMetadata{
					7: {DisplayName: "External Account"},
				},
			},
		}, nil
	}
	t.Cleanup(func() {
		connectSpaceRootRuntimeFunc = prev
	})

	strm := &testSpaceRootRuntimeStream{
		ctx: watchCtx,
		onSend: func(resp *s4wave_root.WatchSpaceRootRuntimeResponse) {
			if resp.GetStatus() == s4wave_root.SpaceRootRuntimeStatus_SpaceRootRuntimeStatus_READY {
				cancel()
			}
		},
	}
	if err := server.WatchSpaceRootRuntime(&s4wave_root.WatchSpaceRootRuntimeRequest{
		AliasId: "company",
	}, strm); err != nil {
		t.Fatal(err)
	}

	if len(strm.sent) != 2 {
		t.Fatalf("sent %d responses, want connecting and ready", len(strm.sent))
	}
	ready := strm.sent[1]
	if ready.GetStatus() != s4wave_root.SpaceRootRuntimeStatus_SpaceRootRuntimeStatus_READY {
		t.Fatalf("second status = %s, want ready", ready.GetStatus())
	}
	if len(ready.GetSessions()) != 1 || ready.GetSessions()[0].GetSessionIndex() != 7 {
		t.Fatalf("sessions = %#v, want session 7", ready.GetSessions())
	}
	if len(ready.GetRuntimeSessions()) != 1 || ready.GetRuntimeSessions()[0].GetMetadata().GetDisplayName() != "External Account" {
		t.Fatalf("runtime sessions = %#v, want enriched session", ready.GetRuntimeSessions())
	}
	records, err := server.snapshotSpaceRootAliases(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].GetAliasId() != "company" {
		t.Fatalf("records after runtime watch = %#v, want unchanged alias", records)
	}
}

func setupSpaceRootAliasServer(
	ctx context.Context,
	t *testing.T,
) (*CoreRootServer, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	tb, err := db_testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()), db_testbed.WithVolumeConfig(
		&volume_kvtxinmem.Config{
			VolumeConfig: &volume_controller.Config{
				VolumeIdAlias: []string{plugin.PluginVolumeID},
			},
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	return NewCoreRootServer(logrus.NewEntry(logrus.New()), tb.Bus), cancel
}

func makeSpaceRootAliasDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

type testSpaceRootRuntimeStream struct {
	ctx    context.Context
	onSend func(*s4wave_root.WatchSpaceRootRuntimeResponse)
	sent   []*s4wave_root.WatchSpaceRootRuntimeResponse
}

func (s *testSpaceRootRuntimeStream) Context() context.Context {
	return s.ctx
}

func (s *testSpaceRootRuntimeStream) MsgSend(srpc.Message) error {
	return nil
}

func (s *testSpaceRootRuntimeStream) MsgRecv(srpc.Message) error {
	return nil
}

func (s *testSpaceRootRuntimeStream) CloseSend() error {
	return nil
}

func (s *testSpaceRootRuntimeStream) Close() error {
	return nil
}

func (s *testSpaceRootRuntimeStream) Send(resp *s4wave_root.WatchSpaceRootRuntimeResponse) error {
	s.sent = append(s.sent, resp.CloneVT())
	if s.onSend != nil {
		s.onSend(resp)
	}
	return nil
}

func (s *testSpaceRootRuntimeStream) SendAndClose(resp *s4wave_root.WatchSpaceRootRuntimeResponse) error {
	if resp != nil {
		return s.Send(resp)
	}
	return nil
}

type testSpaceRootRuntimeRoot struct {
	sessions []*session.SessionListEntry
	metadata map[uint32]*session.SessionMetadata
}

func (r *testSpaceRootRuntimeRoot) WatchSessions(ctx context.Context) (s4wave_root.SRPCRootResourceService_WatchSessionsClient, error) {
	return &testSpaceRootRuntimeWatch{sessions: r.sessions, ctx: ctx}, nil
}

func (r *testSpaceRootRuntimeRoot) MountSessionByIdx(context.Context, uint32) (*s4wave_root.MountSessionByIdxResponse, error) {
	return &s4wave_root.MountSessionByIdxResponse{NotFound: true}, nil
}

func (r *testSpaceRootRuntimeRoot) GetSessionMetadata(_ context.Context, idx uint32) (*session.SessionMetadata, bool, error) {
	meta, ok := r.metadata[idx]
	return meta, !ok, nil
}

type testSpaceRootRuntimeWatch struct {
	sessions []*session.SessionListEntry
	sent     bool
	ctx      context.Context
}

func (w *testSpaceRootRuntimeWatch) Context() context.Context {
	if w.ctx != nil {
		return w.ctx
	}
	return context.Background()
}

func (w *testSpaceRootRuntimeWatch) MsgSend(srpc.Message) error {
	return nil
}

func (w *testSpaceRootRuntimeWatch) MsgRecv(srpc.Message) error {
	return nil
}

func (w *testSpaceRootRuntimeWatch) CloseSend() error {
	return nil
}

func (w *testSpaceRootRuntimeWatch) Close() error {
	return nil
}

func (w *testSpaceRootRuntimeWatch) Recv() (*s4wave_root.WatchSessionsResponse, error) {
	if w.sent {
		<-w.Context().Done()
		return nil, w.Context().Err()
	}
	w.sent = true
	return &s4wave_root.WatchSessionsResponse{Sessions: w.sessions}, nil
}

func (w *testSpaceRootRuntimeWatch) RecvTo(resp *s4wave_root.WatchSessionsResponse) error {
	next, err := w.Recv()
	if err != nil {
		return err
	}
	resp.Sessions = next.GetSessions()
	return nil
}
