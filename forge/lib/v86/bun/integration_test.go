package forge_lib_v86_bun

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_json "github.com/s4wave/spacewave/forge/target/json"
	"github.com/s4wave/spacewave/forge/testbed"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// skipIfNoV86 checks that V86_DIR and V86FS_DIR are set and point to
// valid directories. Skips the test if artifacts are not available.
func skipIfNoV86(t *testing.T) (v86Dir, v86fsDir string) {
	t.Helper()
	v86Dir = os.Getenv("V86_DIR")
	v86fsDir = os.Getenv("V86FS_DIR")
	if v86Dir == "" {
		t.Skip("V86_DIR not set (path to v86 repo with build/, bios/, src/)")
	}
	if v86fsDir == "" {
		t.Skip("V86FS_DIR not set (path to rootfs with bzImage, fs.json, flat/)")
	}
	for _, p := range []string{
		filepath.Join(v86Dir, "build", "v86-debug.wasm"),
		filepath.Join(v86Dir, "bios", "seabios.bin"),
		filepath.Join(v86Dir, "src", "main.ts"),
		filepath.Join(v86fsDir, "bzImage"),
		filepath.Join(v86fsDir, "rootfs.tar"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("required artifact missing: %s", p)
		}
	}
	return v86Dir, v86fsDir
}

// scriptDir returns the absolute path to this package directory.
// boot.ts and v86fs-bridge.ts live here.
func scriptDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := os.Stat(dir + "/boot.ts"); err != nil {
		t.Fatalf("boot.ts not found in %s", dir)
	}
	return dir
}

// TestV86Execution boots a v86 VM through the full forge execution pipeline,
// runs a command that writes to /output, and verifies the output BlockRef
// contains the expected file.
func TestV86Execution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping v86 integration test in short mode")
	}
	v86Dir, v86fsDir := skipIfNoV86(t)
	sdir := scriptDir(t)

	t.Setenv("V86_DIR", v86Dir)
	t.Setenv("V86FS_DIR", v86fsDir)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	rootfsTar := v86fsDir + "/rootfs.tar"
	yaml := `
outputs:
  - name: output
    outputType: OutputType_EXEC
    execOutput: "output"
exec:
  controller:
    id: forge/lib/v86/bun
    config:
      commands:
        - "echo hello-from-v86 > /tmp/output/test.txt"
      output_dir: "/tmp/output"
      script_dir: "` + sdir + `"
      rootfs_tar_path: "` + rootfsTar + `"
      memory_mb: 256
`
	tgt, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(yaml))
	if err != nil {
		t.Fatal(err.Error())
	}

	ts := timestamp.Now()
	valueSet := &forge_target.ValueSet{}

	finalState, err := tb.RunExecutionWithTarget(tgt, valueSet, ts)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Extract output and verify file content.
	outputs := forge_value.ValueSlice(finalState.GetValueSet().GetOutputs())
	valMap, err := outputs.BuildValueMap(true, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	outVal := valMap["output"]
	if outVal == nil || outVal.IsEmpty() {
		t.Fatal("expected output value to be set")
	}

	verifyUnixFSOutput(t, ctx, tb, outVal, "test.txt", []byte("hello-from-v86\n"))
}

// TestV86ExecutionChain runs two v86 executions in sequence. The first
// writes a file to /output. The second mounts the first's output as an
// input, reads the file, and writes a derived file to its own /output.
func TestV86ExecutionChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping v86 integration test in short mode")
	}
	v86Dir, v86fsDir := skipIfNoV86(t)
	sdir := scriptDir(t)

	t.Setenv("V86_DIR", v86Dir)
	t.Setenv("V86FS_DIR", v86fsDir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	ws := tb.WorldState
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	// --- Stage A: write a file to /output ---
	rootfsTar := v86fsDir + "/rootfs.tar"
	yamlA := `
outputs:
  - name: output
    outputType: OutputType_EXEC
    execOutput: "output"
exec:
  controller:
    id: forge/lib/v86/bun
    config:
      commands:
        - "echo stage-a-data > /tmp/output/result.txt"
      output_dir: "/tmp/output"
      script_dir: "` + sdir + `"
      rootfs_tar_path: "` + rootfsTar + `"
      memory_mb: 256
`
	tgtA, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(yamlA))
	if err != nil {
		t.Fatal(err.Error())
	}

	ts := timestamp.Now()
	stateA, err := tb.RunExecutionWithTarget(tgtA, &forge_target.ValueSet{}, ts)
	if err != nil {
		t.Fatalf("stage A: %v", err)
	}

	outputsA := forge_value.ValueSlice(stateA.GetValueSet().GetOutputs())
	valMapA, err := outputsA.BuildValueMap(true, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	outValA := valMapA["output"]
	if outValA == nil || outValA.IsEmpty() {
		t.Fatal("stage A produced no output")
	}
	t.Log("stage A complete, output captured")

	// Store stage A output as a world object so stage B can reference it.
	stageARef := outValA.GetBucketRef()
	stageAObj, err := ws.CreateObject(ctx, "stage-a-output", stageARef)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = stageAObj

	// --- Stage B: mount stage A output, read + derive ---
	yamlB := `
inputs:
  - name: prev
    inputType: InputType_WORLD_OBJECT
    worldObject:
      objectKey: "stage-a-output"
outputs:
  - name: output
    outputType: OutputType_EXEC
    execOutput: "output"
exec:
  controller:
    id: forge/lib/v86/bun
    config:
      commands:
        - "cat /tmp/input/prev/result.txt > /tmp/output/derived.txt"
        - "echo processed-by-stage-b >> /tmp/output/derived.txt"
      mounts:
        /tmp/input/prev: prev
      output_dir: "/tmp/output"
      script_dir: "` + sdir + `"
      rootfs_tar_path: "` + rootfsTar + `"
      memory_mb: 256
`
	tgtB, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(yamlB))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build ValueSet with stage A output as input.
	inpSnapshot, err := forge_value.NewWorldObjectSnapshot(ctx, stageAObj, ws)
	if err != nil {
		t.Fatal(err.Error())
	}
	inpValue := forge_value.NewValueWithWorldObjectSnapshot("prev", inpSnapshot)
	valueSetB := &forge_target.ValueSet{
		Inputs: forge_value.ValueSlice{inpValue},
	}

	stateB, err := tb.RunExecutionWithTarget(tgtB, valueSetB, ts)
	if err != nil {
		t.Fatalf("stage B: %v", err)
	}

	outputsB := forge_value.ValueSlice(stateB.GetValueSet().GetOutputs())
	valMapB, err := outputsB.BuildValueMap(true, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	outValB := valMapB["output"]
	if outValB == nil || outValB.IsEmpty() {
		t.Fatal("stage B produced no output")
	}
	t.Log("stage B complete, output captured")

	// Verify stage B output contains the derived file.
	verifyUnixFSOutput(t, ctx, tb, outValB, "derived.txt", []byte("stage-a-data\nprocessed-by-stage-b\n"))
}

// verifyUnixFSOutput reads a file from a forge output value containing a
// UnixFS tree and checks its content matches expected.
func verifyUnixFSOutput(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	val *forge_value.Value,
	filename string,
	expected []byte,
) {
	t.Helper()

	bref := val.GetBucketRef()
	if bref == nil || bref.GetRootRef().GetEmpty() {
		t.Fatal("output value has no bucket ref")
	}

	objRef := &bucket.ObjectRef{RootRef: bref.GetRootRef()}
	err := tb.WorldState.AccessWorldState(ctx, objRef, func(cs *bucket_lookup.Cursor) error {
		fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cs, nil)
		defer fs.Release()
		fh, err := unixfs.NewFSHandle(fs)
		if err != nil {
			return err
		}
		defer fh.Release()

		bfs := unixfs_billy.NewBillyFS(ctx, fh, "", time.Now())
		data, err := billy_util.ReadFile(bfs, filename)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, expected) {
			t.Errorf("%s: expected %q, got %q", filename, string(expected), string(data))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
}
