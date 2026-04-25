package resource_account_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	resource_account "github.com/s4wave/spacewave/core/resource/account"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
	"github.com/s4wave/spacewave/testbed"
)

// TestWatchAccountInfoLocal verifies the account info watch for a local account.
func TestWatchAccountInfoLocal(t *testing.T) {
	ctx := t.Context()

	tb, _, accountID, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()

	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	displayNameOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpdateDisplayName{
			UpdateDisplayName: &account_settings.UpdateDisplayNameOp{
				DisplayName: "Local Workstation",
			},
		},
	}
	displayNameData, err := displayNameOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, displayNameData)

	keypairOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddEntityKeypair{
			AddEntityKeypair: &session.EntityKeypair{
				PeerId:     "12D3KooWLocalKeypair",
				AuthMethod: "password",
			},
		},
	}
	keypairData, err := keypairOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, keypairData)

	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	rpcCtx, rpcCancel := context.WithCancel(ctx)
	defer rpcCancel()

	var received *s4wave_account.WatchAccountInfoResponse
	strm := &testWatchAccountInfoStream{
		ctx: rpcCtx,
		onSend: func(resp *s4wave_account.WatchAccountInfoResponse) error {
			if resp.GetEntityId() == "Local Workstation" && resp.GetKeypairCount() == 1 {
				received = resp
				rpcCancel()
			}
			return nil
		},
	}

	err = ar.WatchAccountInfo(&s4wave_account.WatchAccountInfoRequest{}, strm)
	if err != nil && rpcCtx.Err() == nil {
		t.Fatal(err)
	}

	if received == nil {
		t.Fatal("expected local account info snapshot")
	}
	if received.GetAccountId() != accountID {
		t.Fatalf("expected account id %q, got %q", accountID, received.GetAccountId())
	}
	if received.GetProviderId() != "local" {
		t.Fatalf("expected provider id local, got %q", received.GetProviderId())
	}
	if received.GetEntityId() != "Local Workstation" {
		t.Fatalf("expected entity id %q, got %q", "Local Workstation", received.GetEntityId())
	}
	if received.GetKeypairCount() != 1 {
		t.Fatalf("expected keypair count 1, got %d", received.GetKeypairCount())
	}
}

// TestWatchSessionsLocal verifies the account sessions watch for a local account.
func TestWatchSessionsLocal(t *testing.T) {
	ctx := t.Context()

	tb, sessRef, _, acc, release := setupLocalProviderAccount(ctx, t)
	defer release()
	sess, sessRelease, err := session.ExMountSession(
		ctx,
		tb.Bus,
		sessRef,
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRelease.Release()

	so, soRelease := mountLocalAccountSettingsSO(ctx, t, tb, acc)
	defer soRelease()

	addOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddPairedDevice{
			AddPairedDevice: &account_settings.PairedDevice{
				PeerId:      "12D3KooWRemotePeer1",
				DisplayName: "Remote Device",
				PairedAt:    1000,
			},
		},
	}
	addOpData, err := addOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, addOpData)

	ar := resource_account.NewAccountResource(acc)
	if ar == nil {
		t.Fatal("expected local account resource")
	}

	rpcCtx, rpcCancel := context.WithCancel(ctx)
	defer rpcCancel()

	currentPeerID := sess.GetPeerId().String()
	if currentPeerID == "" {
		t.Fatal("expected mounted session peer ID")
	}
	currentPresOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpsertSessionPresentation{
			UpsertSessionPresentation: &account_settings.SessionPresentation{
				PeerId:     currentPeerID,
				Label:      "Workstation",
				DeviceType: "desktop",
				ClientName: "Alpha desktop",
				Location:   "Portland, OR",
			},
		},
	}
	currentPresData, err := currentPresOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, currentPresData)
	remotePresOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpsertSessionPresentation{
			UpsertSessionPresentation: &account_settings.SessionPresentation{
				PeerId:     "12D3KooWRemotePeer1",
				ClientName: "Linked device",
				Location:   "Home Office",
			},
		},
	}
	remotePresData, err := remotePresOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	queueAccountSettingsOp(ctx, t, so, remotePresData)

	var received *s4wave_account.WatchSessionsResponse
	strm := &testWatchSessionsStream{
		ctx: rpcCtx,
		onSend: func(resp *s4wave_account.WatchSessionsResponse) error {
			if len(resp.GetSessions()) >= 2 {
				current := resp.GetSessions()[0]
				remote := resp.GetSessions()[1]
				if current.GetLabel() == "Workstation" &&
					current.GetClientName() == "Alpha desktop" &&
					remote.GetClientName() == "Linked device" {
					received = resp
					rpcCancel()
				}
			}
			return nil
		},
	}

	err = ar.WatchSessions(&s4wave_account.WatchSessionsRequest{}, strm)
	if err != nil && rpcCtx.Err() == nil {
		t.Fatal(err)
	}

	if received == nil {
		t.Fatal("expected local sessions snapshot")
	}
	if len(received.GetSessions()) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(received.GetSessions()))
	}

	current := received.GetSessions()[0]
	if current.GetPeerId() != currentPeerID {
		t.Fatalf("expected current peer_id %q, got %q", currentPeerID, current.GetPeerId())
	}
	if !current.GetCurrentSession() {
		t.Fatal("expected current session row to be marked current")
	}
	if current.GetKind() != s4wave_account.AccountSessionKind_AccountSessionKind_ACCOUNT_SESSION_KIND_LOCAL_SESSION {
		t.Fatalf("expected local session kind, got %v", current.GetKind())
	}
	if current.GetLabel() != "Workstation" {
		t.Fatalf("expected current label %q, got %q", "Workstation", current.GetLabel())
	}
	if current.GetClientName() != "Alpha desktop" {
		t.Fatalf("expected current client name %q, got %q", "Alpha desktop", current.GetClientName())
	}
	if current.GetLocation() != "Portland, OR" {
		t.Fatalf("expected current location %q, got %q", "Portland, OR", current.GetLocation())
	}

	remote := received.GetSessions()[1]
	if remote.GetPeerId() != "12D3KooWRemotePeer1" {
		t.Fatalf("expected remote peer_id %q, got %q", "12D3KooWRemotePeer1", remote.GetPeerId())
	}
	if remote.GetCurrentSession() {
		t.Fatal("expected remote session row to be non-current")
	}
	if remote.GetKind() != s4wave_account.AccountSessionKind_AccountSessionKind_ACCOUNT_SESSION_KIND_LOCAL_SESSION {
		t.Fatalf("expected local session kind, got %v", remote.GetKind())
	}
	if remote.GetClientName() != "Linked device" {
		t.Fatalf("expected remote client name %q, got %q", "Linked device", remote.GetClientName())
	}
	if remote.GetLocation() != "Home Office" {
		t.Fatalf("expected remote location %q, got %q", "Home Office", remote.GetLocation())
	}
}

func setupLocalProviderAccount(
	ctx context.Context,
	t *testing.T,
) (*testbed.Testbed, *session.SessionRef, string, *provider_local.ProviderAccount, func()) {
	t.Helper()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	peerID := tb.Volume.GetPeerID()
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     peerID.String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}

	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	localProv := prov.(*provider_local.Provider)
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := localProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	acc := accIface.(*provider_local.ProviderAccount)
	release := func() {
		accRel()
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
	}
	return tb, sessRef, accountID, acc, release
}

func mountLocalAccountSettingsSO(
	ctx context.Context,
	t *testing.T,
	tb *testbed.Testbed,
	acc *provider_local.ProviderAccount,
) (sobject.SharedObject, func()) {
	t.Helper()

	ref, err := acc.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	so, mountRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return so, func() { mountRef.Release() }
}

func queueAccountSettingsOp(
	ctx context.Context,
	t *testing.T,
	so sobject.SharedObject,
	opData []byte,
) {
	t.Helper()

	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		t.Fatal(err)
	}
	if _, wasRejected, err := so.WaitOperation(ctx, localID); err != nil {
		if wasRejected {
			_ = so.ClearOperationResult(ctx, localID)
		}
		t.Fatal(err)
	}
}

type testWatchSessionsStream struct {
	ctx    context.Context
	onSend func(*s4wave_account.WatchSessionsResponse) error
}

func (s *testWatchSessionsStream) Context() context.Context     { return s.ctx }
func (s *testWatchSessionsStream) MsgRecv(_ srpc.Message) error { return nil }
func (s *testWatchSessionsStream) CloseSend() error             { return nil }
func (s *testWatchSessionsStream) Close() error                 { return nil }
func (s *testWatchSessionsStream) MsgSend(_ srpc.Message) error { return nil }
func (s *testWatchSessionsStream) Send(resp *s4wave_account.WatchSessionsResponse) error {
	return s.onSend(resp)
}

func (s *testWatchSessionsStream) SendAndClose(resp *s4wave_account.WatchSessionsResponse) error {
	return s.onSend(resp)
}

type testWatchAccountInfoStream struct {
	ctx    context.Context
	onSend func(*s4wave_account.WatchAccountInfoResponse) error
}

func (s *testWatchAccountInfoStream) Context() context.Context     { return s.ctx }
func (s *testWatchAccountInfoStream) MsgRecv(_ srpc.Message) error { return nil }
func (s *testWatchAccountInfoStream) CloseSend() error             { return nil }
func (s *testWatchAccountInfoStream) Close() error                 { return nil }
func (s *testWatchAccountInfoStream) MsgSend(_ srpc.Message) error { return nil }
func (s *testWatchAccountInfoStream) Send(resp *s4wave_account.WatchAccountInfoResponse) error {
	return s.onSend(resp)
}

func (s *testWatchAccountInfoStream) SendAndClose(resp *s4wave_account.WatchAccountInfoResponse) error {
	return s.onSend(resp)
}
