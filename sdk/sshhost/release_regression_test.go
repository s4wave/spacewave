package s4wave_sshhost

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	"github.com/s4wave/spacewave/testbed"
)

// countingWorldState counts ObjectState releases issued through GetObject.
type countingWorldState struct {
	world.WorldState
	released *int
}

func (c *countingWorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	obj, found, err := c.WorldState.GetObject(ctx, key)
	if err != nil || !found || obj == nil {
		return obj, found, err
	}
	return &countingObjectState{ObjectState: obj, released: c.released}, true, nil
}

// countingObjectState delegates to the wrapped state and counts Release calls.
type countingObjectState struct {
	world.ObjectState
	released *int
}

func (o *countingObjectState) Release() {
	*o.released++
	world.ReleaseObjectState(o.ObjectState)
}

// TestValidateSshHostCredentialSecretsReleasesLookedUpStates fails if a
// body-only Secret lookup leaves its remote-releasable ObjectState alive.
func TestValidateSshHostCredentialSecretsReleasesLookedUpStates(t *testing.T) {
	// Start a testbed for SSH credential state release checks.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Create matching and mismatched SSH credential Secrets.
	createSecretParent(ctx, t, tb.WorldState, "secrets/ssh/key", s4wave_secret.SecretKindSSHPrivateKey)
	createSecretParent(ctx, t, tb.WorldState, "secrets/ssh/wrong", s4wave_secret.SecretKindProviderCredential)

	// Count state releases through the credential lookup World.
	var released int
	ws := &countingWorldState{WorldState: tb.WorldState, released: &released}

	// Validate the matching SSH private key Secret reference.
	refs := &SshHostCredentialRefs{PrivateKeySecretObjectKey: "secrets/ssh/key"}
	if err := ValidateSshHostCredentialSecrets(ctx, ws, refs); err != nil {
		t.Fatalf("ValidateSshHostCredentialSecrets: %v", err)
	}

	// Verify successful validation releases the Secret state.
	if released != 1 {
		t.Fatalf("success path: released %d states, want 1", released)
	}

	// Validate a private key reference to a mismatched Secret kind.
	refs.PrivateKeySecretObjectKey = "secrets/ssh/wrong"
	err = ValidateSshHostCredentialSecrets(ctx, ws, refs)

	// Verify failed validation releases the Secret state.
	if err == nil {
		t.Fatal("expected mismatched SSH credential kind to fail")
	}
	if released != 2 {
		t.Fatalf("error path: released %d states total, want 2", released)
	}
}
