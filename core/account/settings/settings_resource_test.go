//go:build !goscript

package account_settings_test

import (
	"io"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

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
