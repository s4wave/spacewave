package space_world_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
)

// requireDurableGraph walks every block under the payload refs of each stored
// operation above the checkpoint in the Space's durable store, as the account
// replica copy does, and fails on the first block that is not stored.
func requireDurableGraph(ctx context.Context, t *testing.T, phase string, so sobject.SharedObject) int {
	t.Helper()
	var hung []string
	defer func() {
		if len(hung) != 0 {
			t.Logf("HUNG %s: %d reads timed out, first %s", phase, len(hung), hung[0])
		}
	}()
	store := so.GetBlockStore()
	seen := make(map[string]bool)
	var walk func(i int, ref *block.BlockRef, path string)
	walk = func(i int, ref *block.BlockRef, path string) {
		key := ref.MarshalString()
		if seen[key] {
			return
		}
		seen[key] = true
		rctx, rcancel := context.WithTimeout(ctx, 10*time.Second)
		sb, err := store.GetStoredBlock(rctx, ref)
		rcancel()
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			var buf bytes.Buffer
			_ = pprof.Lookup("goroutine").WriteTo(&buf, 2)
			for g := range strings.SplitSeq(buf.String(), "\n\n") {
				if strings.Contains(g, "GetStoredBlock") || strings.Contains(g, "pending-store") || strings.Contains(g, "space/world/adapter") || strings.Contains(g, "bstore") {
					t.Logf("GOROUTINE %s", g)
				}
			}
			hung = append(hung, key)
			return
		}
		if err != nil || sb == nil {
			put, free, swept := block.Stacks(key)
			t.Fatalf("%s: operation %d names %s (path %s), not stored: %v\nput:\n%s\nfree:\n%s\nswept:\n%s",
				phase, i, key, path, err, put, free, swept)
		}
		for _, child := range sb.Refs {
			walk(i, child, path+"/"+child.MarshalString()[:8])
		}
	}
	txs := storedTxs(ctx, t, so)
	for i, tx := range txs {
		refs, err := tx.PayloadRefs(ctx, optypes.LookupWorldOp)
		if err != nil {
			t.Fatalf("%s: operation %d: payload refs: %v", phase, i, err)
		}
		for _, ref := range refs {
			walk(i, ref, tx.GetTxType().String())
		}
	}
	return len(txs)
}

// drainWrite writes what a GLaDOS drain writes: it reads the Worker, creates
// the light on the first call, replaces each light's model block with
// ClearAllRefs and SetBlock, and links it with a graph quad, in one
// transaction.
func drainWrite(ctx context.Context, eng world.Engine, n int) error {
	return world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		if _, err := ws.HasObject(ctx, "worker"); err != nil {
			return err
		}
		obj, found, err := ws.GetObject(ctx, "light")
		if err != nil {
			return err
		}
		if !found {
			obj, err = ws.CreateObject(ctx, "light", nil)
			if err != nil {
				return err
			}
			if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("worker", "governs", "light", "")); err != nil {
				return err
			}
		}
		defer world.ReleaseObjectState(obj)
		root := block_mock.NewExample("light-" + strconv.Itoa(n))
		if _, _, err := world.AccessObjectState(ctx, obj, true, func(cursor *block.Cursor) error {
			cursor.ClearAllRefs()
			cursor.SetBlock(root, true)
			return nil
		}); err != nil {
			return err
		}
		return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("light", "drain", "worker", strconv.Itoa(n)))
	})
}

// seedWorld writes the operations a GLaDOS daemon wrote before its drain: a
// Worker, objects with nested blob roots, and plain model objects.
func seedWorld(ctx context.Context, t *testing.T, eng world.Engine, ops int) {
	t.Helper()
	for i := range ops {
		key := "seed-" + strconv.Itoa(i)
		if i == 0 {
			key = "worker"
		}
		if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
			if i%3 == 1 {
				data := make([]byte, 400<<10)
				_, _ = rand.Read(data)
				obj, _, err := world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
					_, err := blob.BuildBlobWithBytes(ctx, data, bcs)
					return err
				})
				world.ReleaseObjectState(obj)
				return err
			}
			obj, err := ws.CreateObject(ctx, key, nil)
			if err != nil {
				return err
			}
			defer world.ReleaseObjectState(obj)
			_, _, err = world.AccessObjectState(ctx, obj, true, func(cursor *block.Cursor) error {
				cursor.SetBlock(block_mock.NewExample(key), true)
				return nil
			})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// TestHolesDrainStop writes a daemon's operations to a Space that never
// checkpoints, then a drain, and stops the engine at varied points around the
// drain's commit, then remounts, writes the next daemon's start, and walks the
// published graph in the durable store after each step.
func TestHolesDrainStop(t *testing.T) {
	stops := []time.Duration{-1, 0, 50 * time.Microsecond, 200 * time.Microsecond, time.Millisecond, 3 * time.Millisecond, 10 * time.Millisecond, 50 * time.Millisecond}
	for _, store := range []struct {
		name       string
		frees      bool
		maxPending uint64
	}{
		{"local", false, 0},
		{"frees", true, 0},
		{"overflow", false, 64 << 10},
		{"frees-overflow", true, 64 << 10},
	} {
		for _, stop := range stops {
			t.Run(store.name+"/stop-"+stop.String(), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
				defer cancel()
				f := newAdapterFixtureOn(t, fixtureOpts{frees: store.frees, maxPending: store.maxPending, opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
				eng, ref := f.start(t)
				so := f.space(t)
				seedWorld(ctx, t, eng, 10)
				f.collect(t)
				requireDurableGraph(ctx, t, "seeded", so)
				if h := checkpointHeight(ctx, t, so); h != 0 {
					t.Fatalf("checkpoint at %d; the test needs none", h)
				}

				// The drain, with the stop racing its commit, or after it
				// when stop is negative.
				var served *space_world.Engine
				if err := world.AccessEngine(ctx, eng, func(_ context.Context, e world.Engine) error {
					served = e.(*space_world.Engine)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if stop >= 0 {
					go func() {
						time.Sleep(stop)
						ref.Release()
					}()
				}
				err := drainWrite(ctx, eng, 1)
				t.Logf("drain: %v", err)
				if stop < 0 {
					requireDurableGraph(ctx, t, "drained", so)
					ref.Release()
				}
				if _, err := served.WaitSeqno(ctx, ^uint64(0)); !world.IsSuperseded(err) {
					t.Fatalf("waiting for the stopped engine: %v", err)
				}
				f.collect(t)
				n := requireDurableGraph(ctx, t, "stopped", so)

				// The restart: the new daemon clears the drain.
				eng, ref = f.start(t)
				defer func() { ref.Release() }()
				if err := drainWrite(ctx, eng, 2); err != nil {
					t.Fatal(err)
				}
				f.collect(t)
				m := requireDurableGraph(ctx, t, "restarted", so)
				t.Logf("operations: %d after the stop, %d after the restart", n, m)
			})
		}
	}
}

// TestHolesDrainRemote is TestHolesDrainStop through the remote engine a
// GLaDOS daemon uses: the Space engine is served as a resource over an
// in-memory pipe, the drain writes through the SDK engine, and the daemon
// exits (the client closes) right after the drain or racing it, then the
// serve stops at the given delay.
func TestHolesDrainRemote(t *testing.T) {
	type point struct{ exit, stop time.Duration }
	points := []point{{-1, -1}, {0, time.Millisecond}, {0, 20 * time.Millisecond}, {200 * time.Microsecond, 5 * time.Millisecond}, {time.Millisecond, 0}, {3 * time.Millisecond, 10 * time.Millisecond}, {-1, 0}, {-1, 50 * time.Millisecond}}
	for _, store := range []struct {
		name       string
		frees      bool
		maxPending uint64
	}{
		{"local", false, 0},
		{"frees", true, 0},
		{"overflow", false, 64 << 10},
		{"frees-overflow", true, 64 << 10},
	} {
		for _, p := range points {
			t.Run(store.name+"/exit-"+p.exit.String()+"/stop-"+p.stop.String(), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
				defer cancel()
				f := newAdapterFixtureOn(t, fixtureOpts{frees: store.frees, maxPending: store.maxPending, opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
				eng, ref := f.start(t)
				so := f.space(t)

				// Serve the engine to a daemon.
				le := logrus.NewEntry(logrus.New())
				res := resource_world.NewEngineResource(le, f.tb.Bus, eng, optypes.LookupWorldOp, nil)
				client, closeConn := resource_testbed.SetupResourceClientWithRoot(ctx, t, res.GetMux())
				closeClient := sync.OnceFunc(closeConn)
				remote, err := sdk_world_engine.NewSDKEngine(client, client.AccessRootResource())
				if err != nil {
					t.Fatal(err)
				}
				seedWorld(ctx, t, remote, 10)
				if err := drainWrite(ctx, remote, 0); err != nil {
					t.Fatal(err)
				}
				f.collect(t)
				requireDurableGraph(ctx, t, "seeded", so)

				// The drain, the daemon's exit and the serve's stop.
				var served *space_world.Engine
				if err := world.AccessEngine(ctx, eng, func(_ context.Context, e world.Engine) error {
					served = e.(*space_world.Engine)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if p.exit >= 0 {
					go func() {
						time.Sleep(p.exit)
						closeClient()
					}()
				}
				// The serve's stop ends the serving process, so it drops the
				// daemon's connection with the engine. While a daemon stays
				// connected, its Space mount keeps the engine or starts the
				// successor at once; a released engine with a connected
				// client and no successor never occurs.
				if p.stop >= 0 {
					go func() {
						time.Sleep(p.stop)
						ref.Release()
						closeClient()
					}()
				}
				err = drainWrite(ctx, remote, 1)
				t.Logf("drain: %v", err)
				if p.exit < 0 {
					closeClient()
				}
				if p.stop < 0 {
					requireDurableGraph(ctx, t, "drained", so)
					ref.Release()
				}
				if _, err := served.WaitSeqno(ctx, ^uint64(0)); !world.IsSuperseded(err) {
					t.Fatalf("waiting for the stopped engine: %v", err)
				}
				f.collect(t)
				n := requireDurableGraph(ctx, t, "stopped", so)

				// The restart: the new daemon writes again.
				eng, ref = f.start(t)
				defer func() { ref.Release() }()
				if err := drainWrite(ctx, eng, 2); err != nil {
					t.Fatal(err)
				}
				f.collect(t)
				m := requireDurableGraph(ctx, t, "restarted", so)
				t.Logf("operations: %d after the stop, %d after the restart", n, m)
			})
		}
	}
}

// TestHolesReplacedRootInTx writes what a Space's first GLaDOS drain writes:
// one transaction creates a light with a model block root, then replaces that
// root, so the recorded batch names the replaced root in its create entry.
// The durable store must hold every root a stored operation names, with no
// stop at all.
func TestHolesReplacedRootInTx(t *testing.T) {
	for _, store := range []struct {
		name       string
		frees      bool
		maxPending uint64
	}{
		{"local", false, 0},
		{"frees", true, 0},
		{"overflow", false, 64 << 10},
		{"frees-overflow", true, 64 << 10},
	} {
		for _, changelog := range []bool{false, true} {
			for _, remote := range []bool{false, true} {
				name := store.name + "/changelog-" + strconv.FormatBool(changelog) + "/remote-" + strconv.FormatBool(remote)
				t.Run(name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
					defer cancel()
					f := newAdapterFixtureOn(t, fixtureOpts{frees: store.frees, maxPending: store.maxPending, opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
					local, ref := f.start(t)
					defer ref.Release()
					so := f.space(t)
					var eng world.Engine = local
					if remote {
						le := logrus.NewEntry(logrus.New())
						res := resource_world.NewEngineResource(le, f.tb.Bus, local, optypes.LookupWorldOp, nil)
						client, closeClient := resource_testbed.SetupResourceClientWithRoot(ctx, t, res.GetMux())
						defer closeClient()
						sdk, err := sdk_world_engine.NewSDKEngine(client, client.AccessRootResource())
						if err != nil {
							t.Fatal(err)
						}
						eng = sdk
					}
					if err := world.ExecTransaction(ctx, local, true, func(ctx context.Context, ws world.WorldState) error {
						settings := &space_world.SpaceSettings{ChangelogEnabled: changelog}
						_, _, err := space_world_ops.SetSpaceSettings(ctx, ws, "", "", settings, true, time.Now())
						return err
					}); err != nil {
						t.Fatal(err)
					}
					seedWorld(ctx, t, eng, 3)

					// The first drain: create the light, then replace its root.
					if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
						obj, _, err := world.CreateWorldObject(ctx, ws, "light", func(bcs *block.Cursor) error {
							bcs.ClearAllRefs()
							bcs.SetBlock(block_mock.NewExample("light green"), true)
							return nil
						})
						if err != nil {
							return err
						}
						world.ReleaseObjectState(obj)
						if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("worker", "governs", "light", "")); err != nil {
							return err
						}
						obj, _, err = ws.GetObject(ctx, "light")
						if err != nil {
							return err
						}
						defer world.ReleaseObjectState(obj)
						if _, _, err := world.AccessObjectState(ctx, obj, true, func(cursor *block.Cursor) error {
							cursor.ClearAllRefs()
							cursor.SetBlock(block_mock.NewExample("light yellow"), true)
							return nil
						}); err != nil {
							return err
						}
						return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("light", "drain", "worker", ""))
					}); err != nil {
						t.Fatal(err)
					}

					// A create then delete in one transaction names a root the
					// final World never reaches.
					if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
						obj, _, err := world.CreateWorldObject(ctx, ws, "scratch", func(bcs *block.Cursor) error {
							bcs.ClearAllRefs()
							bcs.SetBlock(block_mock.NewExample("scratch"), true)
							return nil
						})
						if err != nil {
							return err
						}
						world.ReleaseObjectState(obj)
						_, err = ws.DeleteObject(ctx, "scratch")
						return err
					}); err != nil {
						t.Fatal(err)
					}
					f.collect(t)
					n := requireDurableGraph(ctx, t, "drained", so)

					// A later write, which may sweep, then the walk again.
					if err := drainWrite(ctx, eng, 2); err != nil {
						t.Fatal(err)
					}
					f.collect(t)
					requireDurableGraph(ctx, t, "later", so)
					t.Logf("operations: %d", n)
				})
			}
		}
	}
}
