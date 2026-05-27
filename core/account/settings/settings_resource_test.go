//go:build !goscript

package account_settings_test

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// TestListPairedDevices verifies the WatchPairedDevices RPC on the session
// resource returns paired devices from the account settings SO.
func TestListPairedDevices(t *testing.T) {
	ctx := t.Context()

	tb, sessRef, accountID, _, release := setupProviderAccount(ctx, t)
	defer release()

	sess, sessRelease, err := session.ExMountSession(ctx, tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRelease.Release()

	le := logrus.NewEntry(logrus.StandardLogger())
	sr := resource_session.NewSessionResource(le, tb.Bus, sess)

	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	defer soRelease()

	addOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddPairedDevice{
			AddPairedDevice: &account_settings.PairedDevice{
				PeerId:      "12D3KooWTestPeer1",
				DisplayName: "Test Device 1",
				PairedAt:    1000,
			},
		},
	}
	addOpData, err := addOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	queueOpAndWaitState(ctx, t, so, addOpData, func(s *account_settings.AccountSettings) bool {
		return len(s.GetPairedDevices()) == 1
	})

	rpcCtx, rpcCancel := context.WithCancel(ctx)
	defer rpcCancel()

	var received *s4wave_session.WatchPairedDevicesResponse
	strm := &testWatchPairedDevicesStream{
		ctx: rpcCtx,
		onSend: func(resp *s4wave_session.WatchPairedDevicesResponse) error {
			if len(resp.GetPairedDevices()) > 0 {
				received = resp
				rpcCancel()
			}
			return nil
		},
	}

	err = sr.WatchPairedDevices(&s4wave_session.WatchPairedDevicesRequest{}, strm)
	if err != nil && rpcCtx.Err() == nil {
		t.Fatal(err)
	}

	if received == nil {
		t.Fatal("expected WatchPairedDevices to return paired devices")
	}
	if len(received.GetPairedDevices()) != 1 {
		t.Fatalf("expected 1 paired device, got %d", len(received.GetPairedDevices()))
	}
	dev := received.GetPairedDevices()[0]
	if dev.GetPeerId() != "12D3KooWTestPeer1" {
		t.Fatalf("expected peer_id %q, got %q", "12D3KooWTestPeer1", dev.GetPeerId())
	}
	if dev.GetDisplayName() != "Test Device 1" {
		t.Fatalf("expected display_name %q, got %q", "Test Device 1", dev.GetDisplayName())
	}
}

// TestLocalSessionAddEntityKeypair verifies the local session resource writes
// entity keypairs through the bound account settings ref.
func TestLocalSessionAddEntityKeypair(t *testing.T) {
	ctx := t.Context()

	tb, sessRef, accountID, _, release := setupProviderAccount(ctx, t)
	defer release()

	sess, sessRelease, err := session.ExMountSession(ctx, tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRelease.Release()

	lsr := resource_session.NewLocalSessionResource(tb.Bus, sess)
	resp, err := lsr.AddEntityKeypair(ctx, &s4wave_session.AddLocalEntityKeypairRequest{
		Credential: &session.EntityCredential{
			Credential: &session.EntityCredential_Password{Password: "test-password"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetPeerId() == "" {
		t.Fatal("expected added entity keypair peer id")
	}

	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	defer soRelease()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relStateCtr()

	var settings *account_settings.AccountSettings
	err = ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			var err error
			settings, err = decodeAccountSettings(ctx, snap)
			if err != nil {
				return err
			}
			if len(settings.GetEntityKeypairs()) == 1 {
				return io.EOF
			}
			return nil
		},
		nil,
	)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if settings == nil {
		t.Fatal("expected account settings state")
	}
	if len(settings.GetEntityKeypairs()) != 1 {
		t.Fatalf("expected 1 entity keypair, got %d", len(settings.GetEntityKeypairs()))
	}
	kp := settings.GetEntityKeypairs()[0]
	if kp.GetPeerId() != resp.GetPeerId() {
		t.Fatalf("expected peer id %q, got %q", resp.GetPeerId(), kp.GetPeerId())
	}
	if kp.GetAuthMethod() != "password" {
		t.Fatalf("expected auth method %q, got %q", "password", kp.GetAuthMethod())
	}
}

// testWatchPairedDevicesStream is a mock stream for testing WatchPairedDevices.
type testWatchPairedDevicesStream struct {
	ctx    context.Context
	onSend func(*s4wave_session.WatchPairedDevicesResponse) error
}

func (s *testWatchPairedDevicesStream) Context() context.Context     { return s.ctx }
func (s *testWatchPairedDevicesStream) MsgRecv(_ srpc.Message) error { return nil }
func (s *testWatchPairedDevicesStream) CloseSend() error             { return nil }
func (s *testWatchPairedDevicesStream) Close() error                 { return nil }
func (s *testWatchPairedDevicesStream) MsgSend(_ srpc.Message) error { return nil }
func (s *testWatchPairedDevicesStream) Send(resp *s4wave_session.WatchPairedDevicesResponse) error {
	return s.onSend(resp)
}

func (s *testWatchPairedDevicesStream) SendAndClose(resp *s4wave_session.WatchPairedDevicesResponse) error {
	return s.onSend(resp)
}
