//go:build !js

package sobject_world_engine_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/pkg/errors"

	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
)

// TestKeypairBlockAtStop stores a keypair through KeypairUpdateOp, whose
// apply sets the object root to a block it staged outside any payload, stops
// the process before any checkpoint (cleanly or by exit), and requires a
// restarted engine to read the keypair back, with StoreEveryWorld on and off.
// With it off the engine keeps non-payload pages in memory only, so the set
// root must travel with the operation's payloads.
func TestKeypairBlockAtStop(t *testing.T) {
	const roleEnv = "SPACEWAVE_KP_STOP_ROLE"
	if role := os.Getenv(roleEnv); role != "" {
		keypairStopWorker(t, role, os.Getenv("SPACEWAVE_KP_STOP_DIR"), os.Getenv("SPACEWAVE_KP_STOP_EVERY") == "1")
		return
	}
	for _, every := range []bool{false, true} {
		for _, stop := range []string{"clean", "crash"} {
			t.Run("every="+strconv.FormatBool(every)+"/"+stop, func(t *testing.T) {
				dir := t.TempDir()
				code := 0
				if stop == "crash" {
					code = 74
				}
				for _, step := range []struct {
					role string
					code int
				}{{"write-" + stop, code}, {"verify", 0}} {
					ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
					cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestKeypairBlockAtStop$", "-test.timeout=50s", "-test.v")
					env := "0"
					if every {
						env = "1"
					}
					cmd.Env = append(os.Environ(), roleEnv+"="+step.role, "SPACEWAVE_KP_STOP_DIR="+dir, "SPACEWAVE_KP_STOP_EVERY="+env)
					out, err := cmd.CombinedOutput()
					cancel()
					if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != step.code {
						t.Errorf("%s: exit %v\n%s", step.role, err, out)
						return
					}
					t.Logf("%s: ok\n%s", step.role, out)
				}
			})
		}
	}
}

// keypairStopWorker runs one step of TestKeypairBlockAtStop on s4db under dir.
func keypairStopWorker(t *testing.T, role, dir string, every bool) {
	ctx := t.Context()
	refPath := filepath.Join(dir, "so-ref")
	keyPath := filepath.Join(dir, "key")
	var soRef *sobject.SharedObjectRef
	if role == "verify" {
		data, err := os.ReadFile(refPath)
		if err != nil {
			t.Fatal(err)
		}
		soRef = &sobject.SharedObjectRef{}
		if err := soRef.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
	}
	storage := filepath.Join(dir, "storage")
	if err := os.MkdirAll(storage, 0o700); err != nil {
		t.Fatal(err)
	}

	// Start the engine with the Space operations.
	tb, _, soRef := openLocalAccount(t, storage, soRef)
	engineID := "kp-stop-engine"
	relOpc, err := tb.Bus.AddController(ctx, world.NewLookupOpController("kp-stop-ops", engineID, optypes.LookupWorldOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relOpc)
	conf := sobject_world_engine.NewConfig(engineID, soRef)
	conf.StoreEveryWorld = every
	eng, engRef, err := world.StartEngine(ctx, tb.Bus, conf, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engRef.Release)

	if role == "verify" {
		key, err := os.ReadFile(keyPath)
		if err != nil {
			t.Fatal(err)
		}
		rctx, rcancel := context.WithTimeout(ctx, 20*time.Second)
		defer rcancel()
		err = world.AccessEngine(rctx, eng, func(ctx context.Context, eng world.Engine) error {
			return world.ExecTransaction(ctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
				body, err := identity_world.LookupKeypairBody(ctx, ws, string(key))
				if err == nil && (body == nil || identity_world.NewKeypairKey(body.GetPeerId()) != string(key)) {
					err = errors.Errorf("keypair %s read back peer %q", key, body.GetPeerId())
				}
				return err
			})
		})
		if err != nil {
			t.Fatalf("verify: keypair read after restart: %v", err)
		}
		return
	}

	// The write must run where the fix matters: with StoreEveryWorld off the
	// engine keeps its pages in memory, so only the operation's payloads
	// carry the set root to the store.
	so, soMountRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, soRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	keeps := sobject_world_engine.KeepsWorldPages(conf, so)
	soMountRef.Release()
	if keeps == every {
		t.Fatalf("%s: engine keeps World pages in memory: %v, want %v", role, keeps, !every)
	}

	// Seed one object, then store a keypair as LookupOrStoreKeypair does.
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "seed", nil)
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, _, pub, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	kp, err := identity.NewKeypair(pub, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	key := identity_world.NewKeypairKey(kp.GetPeerId())
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		_, _, err := identity_world.StoreKeypair(ctx, ws, "", kp, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	data, err := soRef.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: stored keypair %s", role, key)
	if role == "write-crash" {
		os.Exit(74)
	}
}
