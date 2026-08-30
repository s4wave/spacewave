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
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	createSecretParent(ctx, t, tb.WorldState, "secrets/ssh/key", s4wave_secret.SecretKindSSHPrivateKey)
	createSecretParent(ctx, t, tb.WorldState, "secrets/ssh/wrong", s4wave_secret.SecretKindProviderCredential)

	var released int
	ws := &countingWorldState{WorldState: tb.WorldState, released: &released}

	refs := &SshHostCredentialRefs{PrivateKeySecretObjectKey: "secrets/ssh/key"}
	if err := ValidateSshHostCredentialSecrets(ctx, ws, refs); err != nil {
		t.Fatalf("ValidateSshHostCredentialSecrets: %v", err)
	}
	if released != 1 {
		t.Fatalf("success path: released %d states, want 1", released)
	}

	refs.PrivateKeySecretObjectKey = "secrets/ssh/wrong"
	err = ValidateSshHostCredentialSecrets(ctx, ws, refs)
	if err == nil {
		t.Fatal("expected mismatched SSH credential kind to fail")
	}
	if released != 2 {
		t.Fatalf("error path: released %d states total, want 2", released)
	}
}
