package manifest_fetch_world

import (
	"context"
	"io"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/go-git/go-billy/v6/memfs"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestReleaseAuthorizationBeforeStaging exercises the resolver and real filesystem checkout.
func TestReleaseAuthorizationBeforeStaging(t *testing.T) {
	for _, tc := range []struct {
		// name identifies the authorization case.
		name string
		// wantError identifies the required rejection before checkout.
		wantError string
	}{
		{name: "authorized"},
		{name: "missing", wantError: "missing authorization"},
		{name: "forged", wantError: "invalid signature"},
		{name: "wrong signer", wantError: "not a pinned release peer"},
		{name: "changed root", wantError: "does not match authorized root"},
		{name: "direct unsigned manifest", wantError: "missing authorization"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Publish the immutable executable and its independent release authorization.
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			tb := world_testbed.MustDefault(t, ctx)
			ws := world.NewEngineWorldState(tb.Engine, true)
			const storeKey = "release/manifests"
			if _, err := manifest_world.CreateManifestStore(ctx, ws, storeKey); err != nil {
				t.Fatal(err)
			}

			// Generate the release signer independently of the World validators.
			key, _, err := crypto.GenerateEd25519Key(nil)
			if err != nil {
				t.Fatal(err)
			}
			releasePeer, err := peer.IDFromPrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			ref := writeAuthorizedTestManifest(t, ctx, ws, "authorized executable")
			if err := ref.SignReleaseAuthorization(key); err != nil {
				t.Fatal(err)
			}

			// Change only the authorization or selected content for each attack.
			switch tc.name {
			case "missing":
				ref.ReleaseAuthorization = nil
			case "forged":
				ref.ReleaseAuthorization.Signature.SigData[0] ^= 1
			case "wrong signer":
				otherKey, _, err := crypto.GenerateEd25519Key(nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := ref.SignReleaseAuthorization(otherKey); err != nil {
					t.Fatal(err)
				}
			case "changed root":
				ref.ManifestRef = writeAuthorizedTestManifest(t, ctx, ws, "changed executable").GetManifestRef()
			}
			const candidateKey = storeKey + "/core"
			if tc.name == "direct unsigned manifest" {
				if _, _, err := manifest_world.SetManifest(ctx, ws, releasePeer, candidateKey, ref.GetManifestRef()); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name != "direct unsigned manifest" {
				if _, _, err := world.AccessWorldObject(ctx, ws, candidateKey, true, func(cursor *block.Cursor) error {
					cursor.SetBlock(ref, true)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad(storeKey, candidateKey, "core")); err != nil {
				t.Fatal(err)
			}

			// Serve launcher pins through the same plugin RPC path used in production.
			mux := srpc.NewMux()
			if err := manifest.SRPCRegisterReleaseAuthority(mux, &releaseAuthorityTestServer{peers: []string{releasePeer.String()}}); err != nil {
				t.Fatal(err)
			}
			client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
			release, err := tb.Bus.AddHandler(directive.NewFuncHandler(func(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
				dir, ok := inst.GetDirective().(plugin.LoadPlugin)
				if !ok || dir.LoadPluginID() != "launcher" {
					return nil, nil
				}
				return directive.R(directive.NewValueResolver([]plugin.RunningPlugin{plugin.NewRunningPlugin(client)}), nil)
			}))
			if err != nil {
				t.Fatal(err)
			}
			defer release()

			// Resolve the selected root before allowing it to reach the staging filesystem.
			le := logrus.NewEntry(logrus.New())
			ctrl := NewController(le, tb.Bus, &Config{
				EngineId:                 tb.EngineID,
				ObjectKeys:               []string{storeKey},
				DisableWatch:             true,
				ReleaseAuthorityPluginId: "launcher",
			})
			defer ctrl.Close()

			// Resolve through the release authority and selected World.
			handler := &collectionTestHandler{}
			resolver := &fetchManifestResolver{c: ctrl, dir: manifest.NewFetchManifest("core", nil, []string{"js"}, 0)}
			err = resolver.Resolve(ctx, handler)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("authorization error = %v, want %q", err, tc.wantError)
				}
				if len(handler.values) != 0 {
					t.Fatal("unauthorized root reached the staging boundary")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(handler.values) != 1 {
				t.Fatalf("authorized selections = %d, want 1", len(handler.values))
			}

			// Checkout the authorized selection and verify the exact executable bytes.
			selected := handler.values[0].(*manifest.FetchManifestValue).GetManifestRefs()[0]
			dist := memfs.New()
			if _, err := manifest_world.CheckoutManifestToBilly(ctx, le, ws.AccessWorldState, selected.GetManifestRef(), dist, nil, unixfs_sync.DeleteMode_DeleteMode_NONE, nil, nil); err != nil {
				t.Fatal(err)
			}

			// Read the staged executable through the destination filesystem.
			file, err := dist.Open("entrypoint")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "authorized executable" {
				t.Fatalf("staged executable = %q", data)
			}
		})
	}
}

// writeAuthorizedTestManifest creates a complete Manifest and executable filesystem DAG.
func writeAuthorizedTestManifest(t *testing.T, ctx context.Context, ws world.WorldState, contents string) *manifest.ManifestRef {
	// Write the Manifest through the World's real staging storage.
	t.Helper()
	stage, err := ws.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Release()
	meta := &manifest.ManifestMeta{ManifestId: "core", BuildType: "production", PlatformId: "js", Rev: 1}
	ref, err := world.AccessObject(ctx, stage.AccessWorldState, nil, func(cursor *block.Cursor) error {
		return manifest.CreateManifestWithIoFS(ctx, cursor, manifest.NewManifest(meta, "entrypoint"), fstest.MapFS{
			"entrypoint": &fstest.MapFile{Data: []byte(contents), Mode: 0o755},
		}, nil, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifest.NewManifestRef(meta, ref)
}
