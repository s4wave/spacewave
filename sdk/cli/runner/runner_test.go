package runner

import (
	"bytes"
	"context"
	"flag"
	"io"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	provider_pb "github.com/s4wave/spacewave/core/provider"
	session_pb "github.com/s4wave/spacewave/core/session"
	sobject_pb "github.com/s4wave/spacewave/core/sobject"
	space_pb "github.com/s4wave/spacewave/core/space"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

func TestWhoamiCommandUsesInjectedFactory(t *testing.T) {
	var out bytes.Buffer
	factory := &fakeFactory{client: &fakeClient{session: fakeSessionWithSpace("space-123456789", "Alpha")}}
	cmd := NewWhoamiCommand(Config{ClientFactory: factory, Stdout: &out})
	if err := cmd.Action(testContext(t)); err != nil {
		t.Fatalf("run whoami: %v", err)
	}
	if factory.newCalls != 1 {
		t.Fatalf("factory calls = %d", factory.newCalls)
	}
	assertContains(t, out.String(), "Session")
	assertContains(t, out.String(), "session-1")
	assertContains(t, out.String(), "unlocked (auto)")
}

func TestWhoamiCommandWritesJSONOutput(t *testing.T) {
	var out bytes.Buffer
	factory := &fakeFactory{client: &fakeClient{session: fakeSessionWithSpace("space-123456789", "Alpha")}}
	if err := RunWhoami(Config{ClientFactory: factory, Stdout: &out}, testContext(t), "json", 1); err != nil {
		t.Fatalf("run whoami json: %v", err)
	}
	assertContains(t, out.String(), `"sessionId":"session-1"`)
	assertContains(t, out.String(), `"peerId":"peer-1"`)
	assertContains(t, out.String(), `"providerId":"local"`)
	assertContains(t, out.String(), `"providerAccountId":"account-1"`)
	assertContains(t, out.String(), `"lock":"unlocked (auto)"`)
}

func TestSpaceListCommandUsesInjectedFactory(t *testing.T) {
	var out bytes.Buffer
	factory := &fakeFactory{client: &fakeClient{session: fakeSessionWithSpace("space-123456789", "Alpha")}}
	cmd := NewSpaceListCommand(Config{ClientFactory: factory, Stdout: &out}, new(uint))
	if err := cmd.Action(testContext(t)); err != nil {
		t.Fatalf("run space list: %v", err)
	}
	if factory.newCalls != 1 {
		t.Fatalf("factory calls = %d", factory.newCalls)
	}
	assertContains(t, out.String(), "space-12...")
	assertContains(t, out.String(), "Alpha")
}

func TestSpaceListCommandWritesYAMLOutput(t *testing.T) {
	var out bytes.Buffer
	factory := &fakeFactory{client: &fakeClient{session: fakeSessionWithSpace("space-123456789", "Alpha")}}
	if err := RunSpaceList(Config{ClientFactory: factory, Stdout: &out}, testContext(t), "yaml", 1, false); err != nil {
		t.Fatalf("run space list yaml: %v", err)
	}
	assertContains(t, out.String(), "spacesList:")
	assertContains(t, out.String(), "id: space-123456789")
	assertContains(t, out.String(), "name: Alpha")
}

func TestRunStatusReportsMountTimeoutStage(t *testing.T) {
	var out bytes.Buffer
	factory := &fakeFactory{client: &fakeClient{blockMount: true}}
	config := Config{
		ClientFactory: factory,
		Stdout:        &out,
		MountSessionTimeout: func() (time.Duration, error) {
			return time.Millisecond, nil
		},
	}
	err := RunStatus(config, testContext(t), "text", 1)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if factory.newCalls != 1 {
		t.Fatalf("factory calls = %d", factory.newCalls)
	}
	if factory.endpointCalls != 1 {
		t.Fatalf("endpoint calls = %d", factory.endpointCalls)
	}
	assertContains(t, err.Error(), "mount session timed out")
	assertContains(t, out.String(), "Stage")
	assertContains(t, out.String(), "mount session")
}

func testContext(t *testing.T) *cli.Context {
	t.Helper()
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	c := cli.NewContext(nil, set, nil)
	c.Context = context.Background()
	return c
}

func fakeSessionWithSpace(id, name string) *fakeSession {
	return &fakeSession{
		info: &s4wave_session.GetSessionInfoResponse{
			SessionRef: &session_pb.SessionRef{ProviderResourceRef: &provider_pb.ProviderResourceRef{
				Id:                "session-1",
				ProviderId:        "local",
				ProviderAccountId: "account-1",
			}},
			PeerId: "peer-1",
		},
		resources: &fakeResourcesListStream{responses: []*s4wave_session.WatchResourcesListResponse{{
			SpacesList: []*space_pb.SpaceSoListEntry{{
				Entry: &sobject_pb.SharedObjectListEntry{Ref: &sobject_pb.SharedObjectRef{
					ProviderResourceRef: &provider_pb.ProviderResourceRef{Id: id},
				}},
				SpaceMeta: &space_pb.SpaceSoMeta{Name: name},
			}},
		}}},
		lock: &fakeLockStateStream{responses: []*s4wave_session.WatchLockStateResponse{{
			Mode:   session_pb.SessionLockMode_SESSION_LOCK_MODE_AUTO_UNLOCK,
			Locked: false,
		}}},
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !bytes.Contains([]byte(got), []byte(want)) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

type fakeFactory struct {
	client        *fakeClient
	newCalls      int
	endpointCalls int
}

func (f *fakeFactory) NewClient(ctx context.Context, c *cli.Context) (Client, error) {
	f.newCalls++
	return f.client, nil
}

func (f *fakeFactory) StatusEndpoint(ctx context.Context, c *cli.Context) (string, error) {
	f.endpointCalls++
	return "/injected/spacewave.sock", nil
}

type fakeClient struct {
	session    *fakeSession
	blockMount bool
	closed     bool
}

func (c *fakeClient) Close() {
	c.closed = true
}

func (c *fakeClient) MountSession(ctx context.Context, idx uint32) (Session, error) {
	if c.blockMount {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return c.session, nil
}

type fakeSession struct {
	info      *s4wave_session.GetSessionInfoResponse
	resources *fakeResourcesListStream
	lock      *fakeLockStateStream
	released  bool
}

func (s *fakeSession) Release() {
	s.released = true
}

func (s *fakeSession) GetSessionInfo(ctx context.Context) (*s4wave_session.GetSessionInfoResponse, error) {
	return s.info, nil
}

func (s *fakeSession) WatchResourcesList(ctx context.Context) (ResourcesListStream, error) {
	return s.resources, nil
}

func (s *fakeSession) WatchLockState(ctx context.Context) (LockStateStream, error) {
	return s.lock, nil
}

func (s *fakeSession) WatchRecoveryStatus(ctx context.Context) (*s4wave_status.RecoveryStatus, error) {
	return nil, nil
}

type fakeResourcesListStream struct {
	responses []*s4wave_session.WatchResourcesListResponse
	idx       int
	closed    bool
}

func (s *fakeResourcesListStream) Close() error {
	s.closed = true
	return nil
}

func (s *fakeResourcesListStream) Recv() (*s4wave_session.WatchResourcesListResponse, error) {
	resp := s.responses[s.idx]
	s.idx++
	return resp, nil
}

type fakeLockStateStream struct {
	responses []*s4wave_session.WatchLockStateResponse
	idx       int
	closed    bool
}

func (s *fakeLockStateStream) Close() error {
	s.closed = true
	return nil
}

func (s *fakeLockStateStream) Recv() (*s4wave_session.WatchLockStateResponse, error) {
	resp := s.responses[s.idx]
	s.idx++
	return resp, nil
}
