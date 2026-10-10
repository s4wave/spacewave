//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"strings"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// releaseAuthorizationTestKey returns the deterministic fixture release key.
func releaseAuthorizationTestKey(t *testing.T) crypto.PrivKey {
	t.Helper()
	key, _, err := crypto.GenerateEd25519Key(strings.NewReader(strings.Repeat("r", 32)))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// releaseAuthorizationTestPins returns the signer used by the release fixtures.
func releaseAuthorizationTestPins(t *testing.T) []peer.ID {
	t.Helper()
	id, err := peer.IDFromPrivateKey(releaseAuthorizationTestKey(t))
	if err != nil {
		t.Fatal(err)
	}
	return []peer.ID{id}
}

// TestLauncherRejectsUnauthorizedRootsBeforeStaging guards metadata selection outside FetchManifest.
func TestLauncherRejectsUnauthorizedRootsBeforeStaging(t *testing.T) {
	for _, entrypoint := range []string{"native", "CLI"} {
		for _, tc := range []struct {
			// name selects the changed authorization or content.
			name string
			// wantError identifies the rejection at the staging boundary.
			wantError string
		}{
			{name: "missing", wantError: "missing authorization"},
			{name: "forged", wantError: "invalid signature"},
			{name: "wrong signer", wantError: "not a pinned release peer"},
			{name: "changed root", wantError: "does not match authorized root"},
		} {
			t.Run(entrypoint+"/"+tc.name, func(t *testing.T) {
				// Prepare authorized selections for both executable entrypoints.
				ctx, ctrl, metadata, native, cli, _ := buildReleaseMetadataStageUpdateFixture(t, nativeTestPlatformID())
				ref := native
				if entrypoint == "CLI" {
					ref = cli
				}

				// Alter the selected authorization without changing the release pin.
				switch tc.name {
				case "missing":
					ref.ReleaseAuthorization = nil
				case "forged":
					ref.ReleaseAuthorization.Signature.SigData[0] ^= 1
				case "wrong signer":
					key, _, err := crypto.GenerateEd25519Key(nil)
					if err != nil {
						t.Fatal(err)
					}
					if err := ref.SignReleaseAuthorization(key); err != nil {
						t.Fatal(err)
					}
				case "changed root":
					ref.ManifestRef.RootRef.Hash.Hash[0] ^= 1
				}

				// A rejected selection must not allocate any staging filesystem.
				var staged bool
				ctrl.stagingDirFunc = func() (string, error) {
					staged = true
					return t.TempDir(), nil
				}
				err := ctrl.stageReleaseManifestUpdate(ctx, metadata, nativeTestPlatformID(), native, cli)
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("authorization error = %v, want %q", err, tc.wantError)
				}
				if staged {
					t.Fatal("unauthorized root reached the staging filesystem")
				}
			})
		}
	}
}
