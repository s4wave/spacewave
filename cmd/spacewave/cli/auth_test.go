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
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// TestNewAuthMethodAddSubcommandShape pins the names, usage, and required
// flags of the password/pem/backup add-method subcommands so the shared
// addAuthMethodFlow refactor preserves the user-facing CLI surface.
func TestNewAuthMethodAddSubcommandShape(t *testing.T) {
	cmd := newAuthMethodAddCommand()
	if cmd.Name != "add" {
		t.Fatalf("add command name = %q, want %q", cmd.Name, "add")
	}
	want := []struct {
		name  string
		usage string
	}{
		{"password", "add a new password-derived keypair"},
		{"pem", "add a PEM backup key as an auth method"},
		{"backup", "generate a backup key, register it, and save the PEM file"},
	}
	if len(cmd.Subcommands) != len(want) {
		t.Fatalf("add subcommand count = %d, want %d", len(cmd.Subcommands), len(want))
	}
	for i, w := range want {
		sub := cmd.Subcommands[i]
		if sub.Name != w.name {
			t.Errorf("sub[%d].Name = %q, want %q", i, sub.Name, w.name)
		}
		if sub.Usage != w.usage {
			t.Errorf("sub[%d].Usage = %q, want %q", i, sub.Usage, w.usage)
		}
	}
	// pem subcommand must require --file flag.
	pem := cmd.Subcommands[1]
	var fileFlag *cli.StringFlag
	for _, f := range pem.Flags {
		if sf, ok := f.(*cli.StringFlag); ok && sf.Name == "file" {
			fileFlag = sf
			break
		}
	}
	if fileFlag == nil || !fileFlag.Required {
		t.Errorf("pem add subcommand --file flag missing or not required")
	}
}

func TestRunAuthMethodListUsesLocalAccountKeypairs(t *testing.T) {
	restore := stubAuthTestHooks(t)
	defer restore()

	authMountSession = func(ctx context.Context, client *sdkClient, idx uint32) (authSessionHandle, error) {
		if idx != 1 {
			t.Fatalf("unexpected session index: %d", idx)
		}
		return &fakeAuthSessionHandle{info: localAuthSessionInfo()}, nil
	}
	cleanupCalled := false
	authAccessMethodAccount = func(ctx context.Context, client *sdkClient, providerID, accountID string) (authAccountService, func(), error) {
		if providerID != provider_local.ProviderID {
			t.Fatalf("unexpected provider id: %s", providerID)
		}
		if accountID != "local-account" {
			t.Fatalf("unexpected account id: %s", accountID)
		}
		return &fakeAuthMethodAccountService{
			t: t,
			keypairResp: &s4wave_account.WatchEntityKeypairsResponse{
				Keypairs: []*s4wave_account.EntityKeypairState{
					{
						Keypair: &session_pb.EntityKeypair{
							PeerId:     "12D3KooWPasswordKeypair",
							AuthMethod: auth_password.MethodID,
						},
					},
					{
						Keypair: &session_pb.EntityKeypair{
							PeerId:     "12D3KooWBackupPemKeypair",
							AuthMethod: "pem",
						},
					},
				},
			},
		}, func() { cleanupCalled = true }, nil
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
	if !cleanupCalled {
		t.Fatal("account cleanup was not called")
	}
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
	authAccessMethodAccount = func(ctx context.Context, client *sdkClient, providerID, accountID string) (authAccountService, func(), error) {
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
	info    *s4wave_session.GetSessionInfoResponse
	infoErr error
}

func (s *fakeAuthSessionHandle) Release() {}

func (s *fakeAuthSessionHandle) GetSessionInfo(context.Context) (*s4wave_session.GetSessionInfoResponse, error) {
	if s.infoErr != nil {
		return nil, s.infoErr
	}
	return s.info, nil
}

type fakeAuthMethodAccountService struct {
	t              *testing.T
	keypairResp    *s4wave_account.WatchEntityKeypairsResponse
	keypairErr     error
	keypairRecvErr error
}

func (s *fakeAuthMethodAccountService) WatchAuthMethods(
	ctx context.Context,
	req *s4wave_account.WatchAuthMethodsRequest,
) (s4wave_account.SRPCAccountResourceService_WatchAuthMethodsClient, error) {
	s.t.Fatal("unexpected auth methods watch")
	return nil, nil
}

func (s *fakeAuthMethodAccountService) WatchEntityKeypairs(
	ctx context.Context,
	req *s4wave_account.WatchEntityKeypairsRequest,
) (s4wave_account.SRPCAccountResourceService_WatchEntityKeypairsClient, error) {
	if s.keypairErr != nil {
		return nil, s.keypairErr
	}
	return &fakeAccountEntityKeypairsStream{
		ctx:     ctx,
		resp:    s.keypairResp,
		recvErr: s.keypairRecvErr,
	}, nil
}

type fakeAccountEntityKeypairsStream struct {
	ctx     context.Context
	resp    *s4wave_account.WatchEntityKeypairsResponse
	recvErr error
}

func (s *fakeAccountEntityKeypairsStream) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

func (s *fakeAccountEntityKeypairsStream) MsgSend(srpc.Message) error { return nil }

func (s *fakeAccountEntityKeypairsStream) MsgRecv(srpc.Message) error { return nil }

func (s *fakeAccountEntityKeypairsStream) CloseSend() error { return nil }

func (s *fakeAccountEntityKeypairsStream) Close() error { return nil }

func (s *fakeAccountEntityKeypairsStream) Recv() (*s4wave_account.WatchEntityKeypairsResponse, error) {
	if s.recvErr != nil {
		return nil, s.recvErr
	}
	return s.resp, nil
}

func (s *fakeAccountEntityKeypairsStream) RecvTo(m *s4wave_account.WatchEntityKeypairsResponse) error {
	if s.recvErr != nil {
		return s.recvErr
	}
	if s.resp == nil {
		return nil
	}
	*m = *s.resp.CloneVT()
	return nil
}
