//go:build !js

package spacewave_cli

import (
	"context"
	"flag"
	"io"
	"os"
	"testing"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
	auth_password "github.com/s4wave/spacewave/auth/method/password"
	s4wave_provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	session_pb "github.com/s4wave/spacewave/core/session"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

func TestRunAuthMethodListUsesLocalSessionResource(t *testing.T) {
	restore := stubAuthTestHooks(t)
	defer restore()

	authMountSession = func(ctx context.Context, client *sdkClient, idx uint32) (authSessionHandle, error) {
		if idx != 1 {
			t.Fatalf("unexpected session index: %d", idx)
		}
		return &fakeAuthSessionHandle{
			info: localAuthSessionInfo(),
			localSvc: &fakeLocalAuthSessionService{
				resp: &s4wave_session.WatchLocalEntityKeypairsResponse{
					Keypairs: []*session_pb.EntityKeypair{
						{
							PeerId:     "12D3KooWPasswordKeypair",
							AuthMethod: auth_password.MethodID,
						},
						{
							PeerId:     "12D3KooWBackupPemKeypair",
							AuthMethod: "pem",
						},
					},
				},
			},
		}, nil
	}
	authAccessMethodAccount = func(ctx context.Context, client *sdkClient, providerID, accountID string) (authMethodAccountService, func(), error) {
		t.Fatal("unexpected account auth method access for local session")
		return nil, nil, nil
	}

	c := cli.NewContext(nil, emptyFlagSet(t), nil)
	c.Context = context.Background()

	out, err := captureStdout(t, func() error {
		return runAuthMethodList(c, ".spacewave", "text", 1)
	})
	if err != nil {
		t.Fatalf("run auth method list: %v", err)
	}

	assertContains(t, out, "Password")
	assertContains(t, out, "Backup PEM")
	assertContains(t, out, truncateID("12D3KooWPasswordKeypair", 20))
	assertContains(t, out, truncateID("12D3KooWBackupPemKeypair", 20))
}

func TestRunAuthThresholdShowLocalSessionMessage(t *testing.T) {
	restore := stubAuthTestHooks(t)
	defer restore()

	authMountSession = func(ctx context.Context, client *sdkClient, idx uint32) (authSessionHandle, error) {
		return &fakeAuthSessionHandle{info: localAuthSessionInfo()}, nil
	}
	authAccessThresholdAccount = func(ctx context.Context, client *sdkClient, providerID, accountID string) (authThresholdAccountService, func(), error) {
		t.Fatal("unexpected threshold account access for local session")
		return nil, nil, nil
	}

	c := cli.NewContext(nil, emptyFlagSet(t), nil)
	c.Context = context.Background()

	err := runAuthThresholdShow(c, ".spacewave", 1)
	if err == nil {
		t.Fatal("expected local-session threshold show error")
	}
	if err.Error() != localSessionThresholdShowMessage {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAuthThresholdSetLocalSessionMessage(t *testing.T) {
	restore := stubAuthTestHooks(t)
	defer restore()

	authMountSession = func(ctx context.Context, client *sdkClient, idx uint32) (authSessionHandle, error) {
		return &fakeAuthSessionHandle{info: localAuthSessionInfo()}, nil
	}
	authAccessThresholdAccount = func(ctx context.Context, client *sdkClient, providerID, accountID string) (authThresholdAccountService, func(), error) {
		t.Fatal("unexpected threshold account access for local session")
		return nil, nil, nil
	}

	c := cli.NewContext(nil, emptyFlagSet(t), nil)
	c.Context = context.Background()

	err := runAuthThresholdSet(c, ".spacewave", 1, "", 2)
	if err == nil {
		t.Fatal("expected local-session threshold set error")
	}
	if err.Error() != localSessionThresholdSetMessage {
		t.Fatalf("unexpected error: %v", err)
	}
}

func stubAuthTestHooks(t *testing.T) func() {
	t.Helper()

	oldResolveStatePath := authResolveStatePath
	oldConnectDaemon := authConnectDaemon
	oldCloseClient := authCloseClient
	oldMountSession := authMountSession
	oldAccessMethodAccount := authAccessMethodAccount
	oldAccessThresholdAccount := authAccessThresholdAccount

	authResolveStatePath = func(_ *cli.Context, statePath string) (string, error) {
		if statePath != ".spacewave" {
			t.Fatalf("unexpected state path: %s", statePath)
		}
		return "/tmp/state", nil
	}
	authConnectDaemon = func(ctx context.Context, statePath string) (*sdkClient, error) {
		if statePath != "/tmp/state" {
			t.Fatalf("unexpected resolved state path: %s", statePath)
		}
		return &sdkClient{}, nil
	}
	authCloseClient = func(*sdkClient) {}
	authMountSession = func(ctx context.Context, client *sdkClient, idx uint32) (authSessionHandle, error) {
		t.Fatal("authMountSession not stubbed")
		return nil, nil
	}
	authAccessMethodAccount = func(ctx context.Context, client *sdkClient, providerID, accountID string) (authMethodAccountService, func(), error) {
		t.Fatal("authAccessMethodAccount not stubbed")
		return nil, nil, nil
	}
	authAccessThresholdAccount = func(ctx context.Context, client *sdkClient, providerID, accountID string) (authThresholdAccountService, func(), error) {
		t.Fatal("authAccessThresholdAccount not stubbed")
		return nil, nil, nil
	}

	return func() {
		authResolveStatePath = oldResolveStatePath
		authConnectDaemon = oldConnectDaemon
		authCloseClient = oldCloseClient
		authMountSession = oldMountSession
		authAccessMethodAccount = oldAccessMethodAccount
		authAccessThresholdAccount = oldAccessThresholdAccount
	}
}

func localAuthSessionInfo() *s4wave_session.GetSessionInfoResponse {
	return &s4wave_session.GetSessionInfoResponse{
		SessionRef: &session_pb.SessionRef{
			ProviderResourceRef: &s4wave_provider.ProviderResourceRef{
				ProviderId:        provider_local.ProviderID,
				ProviderAccountId: "local-account",
				Id:                "local-session",
			},
		},
	}
}

func emptyFlagSet(t *testing.T) *flag.FlagSet {
	t.Helper()
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return set
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	runErr := fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	return string(data), runErr
}

type fakeAuthSessionHandle struct {
	info     *s4wave_session.GetSessionInfoResponse
	infoErr  error
	localSvc authLocalSessionService
	localErr error
}

func (s *fakeAuthSessionHandle) Release() {}

func (s *fakeAuthSessionHandle) GetSessionInfo(context.Context) (*s4wave_session.GetSessionInfoResponse, error) {
	if s.infoErr != nil {
		return nil, s.infoErr
	}
	return s.info, nil
}

func (s *fakeAuthSessionHandle) AccessLocalSession() (authLocalSessionService, error) {
	if s.localErr != nil {
		return nil, s.localErr
	}
	return s.localSvc, nil
}

type fakeLocalAuthSessionService struct {
	resp    *s4wave_session.WatchLocalEntityKeypairsResponse
	err     error
	recvErr error
}

func (s *fakeLocalAuthSessionService) WatchEntityKeypairs(
	ctx context.Context,
	req *s4wave_session.WatchLocalEntityKeypairsRequest,
) (s4wave_session.SRPCLocalSessionResourceService_WatchEntityKeypairsClient, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &fakeLocalEntityKeypairsStream{
		ctx:     ctx,
		resp:    s.resp,
		recvErr: s.recvErr,
	}, nil
}

type fakeLocalEntityKeypairsStream struct {
	ctx     context.Context
	resp    *s4wave_session.WatchLocalEntityKeypairsResponse
	recvErr error
}

func (s *fakeLocalEntityKeypairsStream) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

func (s *fakeLocalEntityKeypairsStream) MsgSend(srpc.Message) error { return nil }

func (s *fakeLocalEntityKeypairsStream) MsgRecv(srpc.Message) error { return nil }

func (s *fakeLocalEntityKeypairsStream) CloseSend() error { return nil }

func (s *fakeLocalEntityKeypairsStream) Close() error { return nil }

func (s *fakeLocalEntityKeypairsStream) Recv() (*s4wave_session.WatchLocalEntityKeypairsResponse, error) {
	if s.recvErr != nil {
		return nil, s.recvErr
	}
	return s.resp, nil
}

func (s *fakeLocalEntityKeypairsStream) RecvTo(m *s4wave_session.WatchLocalEntityKeypairsResponse) error {
	if s.recvErr != nil {
		return s.recvErr
	}
	if s.resp == nil {
		return nil
	}
	*m = *s.resp.CloneVT()
	return nil
}
