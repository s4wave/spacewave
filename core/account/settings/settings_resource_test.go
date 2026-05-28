package account_settings_test

import (
	"context"
	"crypto/rand"
	"io"
	"runtime"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// TestLocalSessionAddEntityKeypair verifies the local session resource writes
// entity keypairs through the bound account settings ref.
func TestLocalSessionAddEntityKeypair(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("production-cost password scrypt is too slow under GoScript; PEM credential resource coverage runs separately")
	}

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
	kp := waitForSingleEntityKeypair(ctx, t, so)
	if kp.GetPeerId() != resp.GetPeerId() {
		t.Fatalf("expected peer id %q, got %q", resp.GetPeerId(), kp.GetPeerId())
	}
	if kp.GetAuthMethod() != "password" {
		t.Fatalf("expected auth method %q, got %q", "password", kp.GetAuthMethod())
	}
}

func TestLocalSessionAddPEMEntityKeypair(t *testing.T) {
	ctx := t.Context()

	tb, sessRef, accountID, _, release := setupProviderAccount(ctx, t)
	defer release()

	sess, sessRelease, err := session.ExMountSession(ctx, tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRelease.Release()

	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pemData, err := keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		t.Fatal(err)
	}

	lsr := resource_session.NewLocalSessionResource(tb.Bus, sess)
	resp, err := lsr.AddEntityKeypair(ctx, &s4wave_session.AddLocalEntityKeypairRequest{
		Credential: &session.EntityCredential{
			Credential: &session.EntityCredential_PemPrivateKey{PemPrivateKey: pemData},
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

	kp := waitForSingleEntityKeypair(ctx, t, so)
	if kp.GetPeerId() != resp.GetPeerId() {
		t.Fatalf("expected peer id %q, got %q", resp.GetPeerId(), kp.GetPeerId())
	}
	if kp.GetAuthMethod() != "pem" {
		t.Fatalf("expected auth method %q, got %q", "pem", kp.GetAuthMethod())
	}
}

func waitForSingleEntityKeypair(ctx context.Context, t *testing.T, so sobject.SharedObject) *session.EntityKeypair {
	t.Helper()

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
	return settings.GetEntityKeypairs()[0]
}
