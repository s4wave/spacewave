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
	// Skip password-backed resource coverage under the slower GoScript runtime.
	if runtime.GOOS == "js" {
		t.Skip("production-cost password scrypt is too slow under GoScript; PEM credential resource coverage runs separately")
	}

	// Use the test lifecycle context for account and session resources.
	ctx := t.Context()

	// Create the provider account and session used by the local resource.
	tb, sessRef, accountID, _, release := setupProviderAccount(ctx, t)
	defer release()

	// Mount the account session before invoking its local resource.
	sess, sessRelease, err := session.ExMountSession(ctx, tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRelease.Release()

	// Add the password-backed keypair through the local session API.
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

	// Read the persisted account settings keypair from the SharedObject.
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
	// Use the test lifecycle context for the PEM-backed local session resource.
	ctx := t.Context()

	// Create the provider account and session used by the PEM test.
	tb, sessRef, accountID, _, release := setupProviderAccount(ctx, t)
	defer release()

	// Mount the account session before adding its PEM credential.
	sess, sessRelease, err := session.ExMountSession(ctx, tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRelease.Release()

	// Generate the private key used by the PEM credential.
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pemData, err := keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		t.Fatal(err)
	}

	// Add the PEM-backed keypair through the local session API.
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

	// Mount the SharedObject that stores the account settings.
	so, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	defer soRelease()

	// Wait for the account settings SharedObject to publish the added keypair.
	kp := waitForSingleEntityKeypair(ctx, t, so)
	if kp.GetPeerId() != resp.GetPeerId() {
		t.Fatalf("expected peer id %q, got %q", resp.GetPeerId(), kp.GetPeerId())
	}
	if kp.GetAuthMethod() != "pem" {
		t.Fatalf("expected auth method %q, got %q", "pem", kp.GetAuthMethod())
	}
}

func waitForSingleEntityKeypair(ctx context.Context, t *testing.T, so sobject.SharedObject) *session.EntityKeypair {
	// Attribute test failures to the keypair-state caller.
	t.Helper()

	// Open a state controller whose snapshots contain the account settings.
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relStateCtr()

	// Watch snapshots until the single persisted entity keypair is available.
	var settings *account_settings.AccountSettings
	err = ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			// Read the settings of the snapshot.
			var err error
			settings, err = account_settings.ReadSnapshot(ctx, snap)
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
