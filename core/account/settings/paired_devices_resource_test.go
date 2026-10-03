package account_settings_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// TestListPairedDevices verifies the WatchPairedDevices RPC on the session
// resource returns paired devices from the account settings SO.
func TestListPairedDevices(t *testing.T) {
	// Start a provider account whose settings hold the paired devices.
	ctx := t.Context()
	tb, sessRef, accountID, _, release := setupProviderAccount(ctx, t)
	defer release()

	// Mount the account session used by the paired-device RPC.
	sess, sessRelease, err := session.ExMountSession(ctx, tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRelease.Release()

	// Create the session resource serving paired-device watches.
	le := logrus.NewEntry(logrus.StandardLogger())
	sr := resource_session.NewSessionResource(le, tb.Bus, sess)

	// Mount the account settings object for the device addition.
	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	defer soRelease()

	// Encode the device addition for the account settings object.
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

	// Apply the device addition and wait for the settings revision.
	queueOpAndWaitState(ctx, t, so, addOpData, func(s *account_settings.AccountSettings) bool {
		return len(s.GetPairedDevices()) == 1
	})

	// Prepare a watch that cancels after receiving a paired device.
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

	// Run the paired-device watch until its first populated response.
	err = sr.WatchPairedDevices(&s4wave_session.WatchPairedDevicesRequest{}, strm)
	if err != nil && rpcCtx.Err() == nil {
		t.Fatal(err)
	}

	// Verify the watch returns the added device identity and display name.
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
