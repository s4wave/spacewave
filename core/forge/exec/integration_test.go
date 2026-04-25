package space_exec

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// generateTestPeerID creates a random Ed25519 peer ID for testing.
func generateTestPeerID(t *testing.T) peer.ID {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

// setupIntegrationTest creates a testbed with world state, a peer ID,
// a default registry, and a logger.
func setupIntegrationTest(t *testing.T) (context.Context, world.WorldState, peer.ID, *Registry, *logrus.Entry) {
	t.Helper()
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pid := generateTestPeerID(t)
	le := logrus.NewEntry(logrus.StandardLogger())
	registry := NewDefaultRegistry()
	return ctx, tb.WorldState, pid, registry, le
}

// createTestFS initializes a unixfs FS_NODE object with a single file.
func createTestFS(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey, fileName string,
	data []byte,
) {
	t.Helper()
	_, _, err := unixfs_world.FsInit(
		ctx, ws, sender, objKey,
		unixfs_world.FSType_FSType_FS_NODE,
		nil, false, time.Now(),
	)
	if err != nil {
		t.Fatalf("FsInit: %v", err)
	}
	obj, err := world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		t.Fatalf("MustGetObject: %v", err)
	}
	_, _, err = unixfs_world.FsMknodWithContent(
		ctx, obj, sender,
		unixfs_world.FSType_FSType_FS_NODE,
		[]string{fileName},
		unixfs.NewFSCursorNodeType_File(),
		int64(len(data)),
		bytes.NewReader(data),
		0o644,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("FsMknodWithContent: %v", err)
	}
}

// createTestExecution creates a pending forge execution object targeting
// the given handler config ID.
func createTestExecution(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	execKey, configID string,
	configData []byte,
) {
	t.Helper()
	tgt := &forge_target.Target{
		Exec: &forge_target.Exec{
			Controller: &configset_proto.ControllerConfig{
				Id:     configID,
				Rev:    1,
				Config: configData,
			},
		},
	}
	_, err := forge_execution.CreateExecutionWithTarget(
		ctx, ws, sender, execKey, sender, nil, tgt, timestamp.Now(),
	)
	if err != nil {
		t.Fatalf("CreateExecutionWithTarget: %v", err)
	}
}

// mustReadExecution reads back an execution from world state.
func mustReadExecution(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	execKey string,
) *forge_execution.Execution {
	t.Helper()
	ex, _, err := forge_execution.LookupExecution(ctx, ws, execKey)
	if err != nil {
		t.Fatalf("LookupExecution: %v", err)
	}
	return ex
}

// assertComplete checks that the execution completed successfully.
func assertComplete(t *testing.T, ex *forge_execution.Execution) {
	t.Helper()
	if !ex.IsComplete() {
		t.Fatalf("expected COMPLETE, got %s", ex.GetExecutionState().String())
	}
	if !ex.GetResult().IsSuccessful() {
		t.Fatalf("expected success, got error: %s", ex.GetResult().GetFailError())
	}
}

// findOutput looks up a named output value from the execution's ValueSet.
func findOutput(ex *forge_execution.Execution, name string) *forge_value.Value {
	for _, out := range ex.GetValueSet().GetOutputs() {
		if out.GetName() == name {
			return out
		}
	}
	return nil
}

// findLogContaining returns the first log entry whose message contains substr.
func findLogContaining(ex *forge_execution.Execution, substr string) *forge_execution.LogEntry {
	for _, entry := range ex.GetLogEntries() {
		if strings.Contains(entry.GetMessage(), substr) {
			return entry
		}
	}
	return nil
}

// TestIntegration_Noop runs a noop execution through the full
// PENDING -> RUNNING -> COMPLETE lifecycle.
func TestIntegration_Noop(t *testing.T) {
	ctx, ws, pid, registry, le := setupIntegrationTest(t)

	execKey := "exec/noop-int"
	createTestExecution(t, ctx, ws, pid, execKey, NoopConfigID, nil)

	if err := ProcessExecution(ctx, le, ws, registry, execKey, pid); err != nil {
		t.Fatalf("ProcessExecution: %v", err)
	}

	ex := mustReadExecution(t, ctx, ws, execKey)
	assertComplete(t, ex)
	if entry := findLogContaining(ex, "noop execution complete"); entry == nil {
		t.Fatal("expected noop execution log entry")
	}
}

// TestIntegration_UnixfsRead creates a unixfs object with a test file,
// runs the unixfs-read handler, and verifies the output snapshot.
func TestIntegration_UnixfsRead(t *testing.T) {
	ctx, ws, pid, registry, le := setupIntegrationTest(t)

	fsKey := "fs/int-read"
	content := []byte("hello world from unixfs")
	createTestFS(t, ctx, ws, pid, fsKey, "hello.txt", content)

	execKey := "exec/unixfs-read-int"
	config := []byte(`{"object_key":"fs/int-read","file_path":"hello.txt"}`)
	createTestExecution(t, ctx, ws, pid, execKey, UnixfsReadConfigID, config)

	if err := ProcessExecution(ctx, le, ws, registry, execKey, pid); err != nil {
		t.Fatalf("ProcessExecution: %v", err)
	}

	ex := mustReadExecution(t, ctx, ws, execKey)
	assertComplete(t, ex)

	// Log should mention bytes read.
	if entry := findLogContaining(ex, "bytes from "+fsKey); entry == nil {
		t.Fatal("expected log entry with file read info")
	}

	// Output should be a WORLD_OBJECT_SNAPSHOT named "source".
	out := findOutput(ex, "source")
	if out == nil {
		t.Fatal("expected output named 'source'")
	}
	if out.GetValueType() != forge_value.ValueType_ValueType_WORLD_OBJECT_SNAPSHOT {
		t.Fatalf("expected WORLD_OBJECT_SNAPSHOT, got %s", out.GetValueType().String())
	}
}

// TestIntegration_FileHash creates a unixfs object, runs the file-hash
// handler, and verifies the blake3 digest in the log.
func TestIntegration_FileHash(t *testing.T) {
	ctx, ws, pid, registry, le := setupIntegrationTest(t)

	fsKey := "fs/int-hash"
	content := []byte("hash me please")
	createTestFS(t, ctx, ws, pid, fsKey, "data.bin", content)

	execKey := "exec/file-hash-int"
	config := []byte(`{"object_key":"fs/int-hash","file_path":"data.bin"}`)
	createTestExecution(t, ctx, ws, pid, execKey, FileHashConfigID, config)

	if err := ProcessExecution(ctx, le, ws, registry, execKey, pid); err != nil {
		t.Fatalf("ProcessExecution: %v", err)
	}

	ex := mustReadExecution(t, ctx, ws, execKey)
	assertComplete(t, ex)

	// Compute expected blake3 hash.
	hasher := blake3.New()
	_, _ = hasher.Write(content)
	expectedDigest := hex.EncodeToString(hasher.Sum(nil))

	// Log should contain the digest.
	if entry := findLogContaining(ex, "blake3:"+expectedDigest); entry == nil {
		var msgs []string
		for _, e := range ex.GetLogEntries() {
			msgs = append(msgs, e.GetMessage())
		}
		t.Fatalf("expected log with blake3:%s, got: %v", expectedDigest, msgs)
	}
}

// TestIntegration_ExportZip creates a unixfs object, runs the export-zip
// handler, and verifies the zip blob output reference.
func TestIntegration_ExportZip(t *testing.T) {
	ctx, ws, pid, registry, le := setupIntegrationTest(t)

	fsKey := "fs/int-zip"
	content := []byte("zip this content")
	createTestFS(t, ctx, ws, pid, fsKey, "readme.txt", content)

	execKey := "exec/export-zip-int"
	config := []byte(`{"object_key":"fs/int-zip"}`)
	createTestExecution(t, ctx, ws, pid, execKey, ExportZipConfigID, config)

	if err := ProcessExecution(ctx, le, ws, registry, execKey, pid); err != nil {
		t.Fatalf("ProcessExecution: %v", err)
	}

	ex := mustReadExecution(t, ctx, ws, execKey)
	assertComplete(t, ex)

	// Log should mention zip bytes.
	if entry := findLogContaining(ex, "zip:"); entry == nil {
		t.Fatal("expected log entry with zip info")
	}

	// Output should be a BUCKET_REF named "zip".
	out := findOutput(ex, "zip")
	if out == nil {
		t.Fatal("expected output named 'zip'")
	}
	if out.GetValueType() != forge_value.ValueType_ValueType_BUCKET_REF {
		t.Fatalf("expected BUCKET_REF, got %s", out.GetValueType().String())
	}
}
