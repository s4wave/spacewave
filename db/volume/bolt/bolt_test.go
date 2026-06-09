//go:build !js && !wasip1

package volume_bolt_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/core"
	"github.com/s4wave/spacewave/db/volume"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_test "github.com/s4wave/spacewave/db/volume/test"
	"github.com/sirupsen/logrus"
)

const (
	boltBlockVisibilityRoleEnv    = "SPACEWAVE_BOLT_BLOCK_VISIBILITY_ROLE"
	boltBlockVisibilityPathEnv    = "SPACEWAVE_BOLT_BLOCK_VISIBILITY_PATH"
	boltBlockVisibilityRefsDirEnv = "SPACEWAVE_BOLT_BLOCK_VISIBILITY_REFS_DIR"
	boltBlockVisibilityIDEnv      = "SPACEWAVE_BOLT_BLOCK_VISIBILITY_ID"
	boltBlockVisibilityItersEnv   = "SPACEWAVE_BOLT_BLOCK_VISIBILITY_ITERS"
	boltBlockVisibilityWritersEnv = "SPACEWAVE_BOLT_BLOCK_VISIBILITY_WRITERS"

	boltGraphRoleEnv    = "SPACEWAVE_BOLT_GRAPH_ROLE"
	boltGraphPathEnv    = "SPACEWAVE_BOLT_GRAPH_PATH"
	boltGraphRefsDirEnv = "SPACEWAVE_BOLT_GRAPH_REFS_DIR"
	boltGraphIDEnv      = "SPACEWAVE_BOLT_GRAPH_ID"
	boltGraphItersEnv   = "SPACEWAVE_BOLT_GRAPH_ITERS"
	boltGraphWritersEnv = "SPACEWAVE_BOLT_GRAPH_WRITERS"
	boltGraphReadyEnv   = "SPACEWAVE_BOLT_GRAPH_READY_PATH"
	boltGraphDoneEnv    = "SPACEWAVE_BOLT_GRAPH_DONE_PATH"
	boltGraphGCEnv      = "SPACEWAVE_BOLT_GRAPH_GC"
)

// TestBoltVolume tests the bolt-backed volume including storage stats.
func TestBoltVolume(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	sr.AddFactory(volume_bolt.NewFactory(b))

	tempDir, err := os.MkdirTemp("", "bolt_test_*")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(tempDir)

	path := filepath.Join(tempDir, "test.db")
	volCtrl, _, diRef, err := loader.WaitExecControllerRunningTyped[volume.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&volume_bolt.Config{Path: path}),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef.Release()

	bvol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	if err := volume_test.CheckVolume(ctx, bvol); err != nil {
		t.Fatal(err.Error())
	}

	if err := volume_test.CheckStorageStatsNonZero(ctx, bvol); err != nil {
		t.Fatal(err.Error())
	}

	capability, err := bvol.Capability(ctx, coord.Scope{
		VolumeID:      bvol.GetID(),
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if !capability.Supported {
		t.Fatal("expected bbolt coordination capability to be supported")
	}
	if capability.Backend != coord.BackendKindBbolt {
		t.Fatalf("coord backend = %q, want bbolt", capability.Backend)
	}
}

// TestBoltVolumeSyncsFreelistByDefault verifies bolt volumes use bbolt's
// multi-process-safe freelist mode.
func TestBoltVolumeSyncsFreelistByDefault(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	sr.AddFactory(volume_bolt.NewFactory(b))

	tempDir, err := os.MkdirTemp("", "bolt_test_*")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(tempDir)

	path := filepath.Join(tempDir, "test.db")
	volCtrl, _, diRef, err := loader.WaitExecControllerRunningTyped[volume.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&volume_bolt.Config{Path: path}),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef.Release()

	bvol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	bdb := volume_bolt.GetBoltDB(bvol)
	if bdb == nil {
		t.Fatal("expected bolt-backed volume")
	}
	if bdb.NoFreelistSync {
		t.Fatal("expected synced freelist for multi-process bbolt access")
	}
	if bdb.NoSync {
		t.Fatal("expected synced writes for multi-process bbolt access")
	}
}

func TestBoltVolumeMultiprocessBlockVisibility(t *testing.T) {
	if role := os.Getenv(boltBlockVisibilityRoleEnv); role != "" {
		runBoltBlockVisibilityRole(t, role)
		return
	}

	dir := t.TempDir()
	boltPath := filepath.Join(dir, "blocks.db")
	refsDir := filepath.Join(dir, "refs")
	if err := os.Mkdir(refsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	vol := openBoltVisibilityVolume(t, boltPath)
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	const writers = 3
	const iterations = 20
	cmds := make([]*exec.Cmd, 0, writers)
	for id := range writers {
		cmds = append(cmds, boltBlockVisibilityCommand(t, "writer", boltPath, refsDir, id, iterations, writers))
	}
	for _, cmd := range cmds {
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("writer failed: %v\n%s", err, cmdOutput(cmd))
		}
	}

	verifier := boltBlockVisibilityCommand(t, "verifier", boltPath, refsDir, 0, iterations, writers)
	if err := verifier.Run(); err != nil {
		t.Fatalf("verifier failed: %v\n%s", err, cmdOutput(verifier))
	}
}

func TestBoltVolumeMultiprocessScopedGraphVisibility(t *testing.T) {
	if role := os.Getenv(boltGraphRoleEnv); role != "" {
		runBoltGraphRole(t, role)
		return
	}
	runBoltGraphVisibilityParent(t, false)
}

func TestBoltVolumeMultiprocessScopedGCGraphVisibility(t *testing.T) {
	if role := os.Getenv(boltGraphRoleEnv); role != "" {
		runBoltGraphRole(t, role)
		return
	}
	runBoltGraphVisibilityParent(t, true)
}

func runBoltGraphVisibilityParent(t *testing.T, useGC bool) {
	t.Helper()

	dir := t.TempDir()
	boltPath := filepath.Join(dir, "graphs.db")
	refsDir := filepath.Join(dir, "refs")
	readyPath := filepath.Join(dir, "reader-ready")
	donePath := filepath.Join(dir, "writers-done")
	if err := os.Mkdir(refsDir, 0o700); err != nil {
		t.Fatal(err)
	}

	vol := openBoltVisibilityVolume(t, boltPath)
	initialRef := writeBoltGraph(t, context.Background(), vol, "initial", useGC)
	if err := os.WriteFile(filepath.Join(refsDir, "initial.ref"), []byte(initialRef.MarshalString()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	reader := boltGraphCommand(t, "reader", boltPath, refsDir, readyPath, donePath, 0, 0, 0)
	if useGC {
		reader.Env = append(reader.Env, boltGraphGCEnv+"=1")
	}
	if err := reader.Start(); err != nil {
		t.Fatal(err)
	}
	waitForPath(t, readyPath)

	const writers = 2
	const iterations = 12
	cmds := make([]*exec.Cmd, 0, writers)
	for id := range writers {
		cmd := boltGraphCommand(t, "writer", boltPath, refsDir, readyPath, donePath, id, iterations, writers)
		if useGC {
			cmd.Env = append(cmd.Env, boltGraphGCEnv+"=1")
		}
		cmds = append(cmds, cmd)
	}
	for _, cmd := range cmds {
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("writer failed: %v\n%s", err, cmdOutput(cmd))
		}
	}
	if err := os.WriteFile(donePath, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := reader.Wait(); err != nil {
		t.Fatalf("reader failed: %v\n%s", err, cmdOutput(reader))
	}

	verifier := boltGraphCommand(t, "verifier", boltPath, refsDir, readyPath, donePath, 0, iterations, writers)
	if useGC {
		verifier.Env = append(verifier.Env, boltGraphGCEnv+"=1")
	}
	if err := verifier.Run(); err != nil {
		t.Fatalf("verifier failed: %v\n%s", err, cmdOutput(verifier))
	}
}

func boltBlockVisibilityCommand(
	t *testing.T,
	role string,
	boltPath string,
	refsDir string,
	id int,
	iterations int,
	writers int,
) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestBoltVolumeMultiprocessBlockVisibility$", "-test.v")
	cmd.Env = append(os.Environ(),
		boltBlockVisibilityRoleEnv+"="+role,
		boltBlockVisibilityPathEnv+"="+boltPath,
		boltBlockVisibilityRefsDirEnv+"="+refsDir,
		boltBlockVisibilityIDEnv+"="+strconv.Itoa(id),
		boltBlockVisibilityItersEnv+"="+strconv.Itoa(iterations),
		boltBlockVisibilityWritersEnv+"="+strconv.Itoa(writers),
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	return cmd
}

func runBoltBlockVisibilityRole(t *testing.T, role string) {
	t.Helper()

	id, err := strconv.Atoi(os.Getenv(boltBlockVisibilityIDEnv))
	if err != nil {
		t.Fatal(err)
	}
	iterations, err := strconv.Atoi(os.Getenv(boltBlockVisibilityItersEnv))
	if err != nil {
		t.Fatal(err)
	}
	writers, err := strconv.Atoi(os.Getenv(boltBlockVisibilityWritersEnv))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	vol := openBoltVisibilityVolume(t, os.Getenv(boltBlockVisibilityPathEnv))
	defer vol.Close()

	switch role {
	case "writer":
		for i := range iterations {
			data := boltVisibilityData(id, i)
			ref, _, err := vol.PutBlock(ctx, data, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(boltVisibilityRefPath(id, i), []byte(ref.MarshalString()), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	case "verifier":
		for writerID := range writers {
			for i := range iterations {
				refData, err := os.ReadFile(boltVisibilityRefPath(writerID, i))
				if err != nil {
					t.Fatal(err)
				}
				ref, err := block.UnmarshalBlockRefB58(string(refData))
				if err != nil {
					t.Fatal(err)
				}
				data, found, err := vol.GetBlock(ctx, ref)
				if err != nil {
					t.Fatal(err)
				}
				if !found {
					t.Fatalf("missing block writer=%d iteration=%d ref=%s", writerID, i, ref.MarshalString())
				}
				if string(data) != string(boltVisibilityData(writerID, i)) {
					t.Fatalf("block data mismatch writer=%d iteration=%d: %q", writerID, i, data)
				}
			}
		}
	default:
		t.Fatalf("unknown role %q", role)
	}
}

func boltGraphCommand(
	t *testing.T,
	role string,
	boltPath string,
	refsDir string,
	readyPath string,
	donePath string,
	id int,
	iterations int,
	writers int,
) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestBoltVolumeMultiprocessScopedGraphVisibility$", "-test.v")
	cmd.Env = append(os.Environ(),
		boltGraphRoleEnv+"="+role,
		boltGraphPathEnv+"="+boltPath,
		boltGraphRefsDirEnv+"="+refsDir,
		boltGraphReadyEnv+"="+readyPath,
		boltGraphDoneEnv+"="+donePath,
		boltGraphIDEnv+"="+strconv.Itoa(id),
		boltGraphItersEnv+"="+strconv.Itoa(iterations),
		boltGraphWritersEnv+"="+strconv.Itoa(writers),
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	return cmd
}

func runBoltGraphRole(t *testing.T, role string) {
	t.Helper()

	id, err := strconv.Atoi(os.Getenv(boltGraphIDEnv))
	if err != nil {
		t.Fatal(err)
	}
	iterations, err := strconv.Atoi(os.Getenv(boltGraphItersEnv))
	if err != nil {
		t.Fatal(err)
	}
	writers, err := strconv.Atoi(os.Getenv(boltGraphWritersEnv))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	vol := openBoltVisibilityVolume(t, os.Getenv(boltGraphPathEnv))
	defer vol.Close()
	useGC := os.Getenv(boltGraphGCEnv) == "1"

	switch role {
	case "reader":
		refData, err := os.ReadFile(filepath.Join(os.Getenv(boltGraphRefsDirEnv), "initial.ref"))
		if err != nil {
			t.Fatal(err)
		}
		ref, err := block.UnmarshalBlockRefB58(string(refData))
		if err != nil {
			t.Fatal(err)
		}
		store := boltGraphStore(vol, useGC)
		scoped, release, err := store.BeginReadOperation(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if err := readBoltGraph(ctx, scoped, ref, "initial"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv(boltGraphReadyEnv), []byte("ready"), 0o600); err != nil {
			t.Fatal(err)
		}
		for range 200 {
			if err := readBoltGraph(ctx, scoped, ref, "initial"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(os.Getenv(boltGraphDoneEnv)); err == nil {
				return
			}
		}
	case "writer":
		for i := range iterations {
			label := fmt.Sprintf("writer/%d/%03d", id, i)
			ref := writeBoltGraph(t, ctx, vol, label, useGC)
			if err := os.WriteFile(boltGraphRefPath(id, i), []byte(ref.MarshalString()), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	case "verifier":
		for writerID := range writers {
			for i := range iterations {
				label := fmt.Sprintf("writer/%d/%03d", writerID, i)
				refData, err := os.ReadFile(boltGraphRefPath(writerID, i))
				if err != nil {
					t.Fatal(err)
				}
				ref, err := block.UnmarshalBlockRefB58(string(refData))
				if err != nil {
					t.Fatal(err)
				}
				if err := readBoltGraph(ctx, boltGraphStore(vol, useGC), ref, label); err != nil {
					t.Fatal(err)
				}
			}
		}
	default:
		t.Fatalf("unknown role %q", role)
	}
}

func writeBoltGraph(t *testing.T, ctx context.Context, vol *volume_bolt.Bolt, label string, useGC bool) *block.BlockRef {
	t.Helper()

	store := boltGraphStore(vol, useGC)
	tx, cursor := block.NewTransaction(store, nil, nil, nil)
	root := &block_mock.Root{ExampleSubBlock: &block_mock.SubBlock{}}
	cursor.SetBlock(root, true)
	child := cursor.FollowSubBlock(1).FollowRef(1, nil)
	child.SetBlock(block_mock.NewExample(label), true)
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if gcOps, ok := store.(*block_gc.GCStoreOps); ok {
		if err := gcOps.FlushPending(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return ref
}

func boltGraphStore(vol *volume_bolt.Bolt, useGC bool) block.StoreOps {
	if !useGC {
		return vol
	}
	return block_gc.NewGCStoreOps(vol, vol.GetRefGraph())
}

func readBoltGraph(ctx context.Context, store block.StoreOps, ref *block.BlockRef, want string) error {
	_, cursor := block.NewTransaction(store, nil, ref, nil)
	root, err := block.UnmarshalBlock[*block_mock.Root](ctx, cursor, block_mock.NewRootBlock)
	if err != nil {
		return err
	}
	if root == nil || root.GetExampleSubBlock() == nil {
		return fmt.Errorf("graph root missing sub block")
	}
	exampleCursor := cursor.FollowSubBlock(1).FollowRef(1, root.GetExampleSubBlock().GetExamplePtr())
	example, err := block_mock.UnmarshalExample(ctx, exampleCursor)
	if err != nil {
		return err
	}
	if example == nil || example.GetMsg() != want {
		return fmt.Errorf("graph example = %#v, want %q", example, want)
	}
	return nil
}

func boltGraphRefPath(writerID int, iteration int) string {
	return filepath.Join(
		os.Getenv(boltGraphRefsDirEnv),
		fmt.Sprintf("writer-%d-%03d.ref", writerID, iteration),
	)
}

func openBoltVisibilityVolume(t *testing.T, boltPath string) *volume_bolt.Bolt {
	t.Helper()

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	vol, err := volume_bolt.NewBolt(context.Background(), logrus.NewEntry(log), &volume_bolt.Config{
		Path: boltPath,
		VolumeConfig: &volume_controller.Config{
			GcIntervalDur: "0",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return vol
}

func boltVisibilityData(writerID int, iteration int) []byte {
	return []byte(fmt.Sprintf("writer/%d/%03d", writerID, iteration))
}

func boltVisibilityRefPath(writerID int, iteration int) string {
	return filepath.Join(
		os.Getenv(boltBlockVisibilityRefsDirEnv),
		fmt.Sprintf("writer-%d-%03d.ref", writerID, iteration),
	)
}

func waitForPath(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func cmdOutput(cmd *exec.Cmd) string {
	if buf, ok := cmd.Stdout.(*bytes.Buffer); ok {
		return buf.String()
	}
	return ""
}
