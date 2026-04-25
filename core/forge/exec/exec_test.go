package space_exec

import (
	"context"
	"testing"

	git_block "github.com/s4wave/spacewave/db/git/block"
)

func TestNoopHandler(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterNoop(r)

	// Lookup returns the factory.
	factory := r.Lookup(NoopConfigID)
	if factory == nil {
		t.Fatal("noop factory not found")
	}

	// CreateHandler returns a handler.
	handler, err := r.CreateHandler(ctx, nil, nil, nil, nil, NoopConfigID, nil)
	if err != nil {
		t.Fatalf("CreateHandler: %v", err)
	}

	// Execute returns nil (completion).
	if err := handler.Execute(ctx); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestRegistryUnknownID(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()

	_, err := r.CreateHandler(ctx, nil, nil, nil, nil, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown config ID")
	}
}

func TestKvtxHandlerRegistration(t *testing.T) {
	r := NewRegistry()
	RegisterKvtx(r)

	factory := r.Lookup(KvtxConfigID)
	if factory == nil {
		t.Fatal("kvtx factory not found in registry")
	}
	if KvtxConfigID == "" {
		t.Fatal("KvtxConfigID is empty")
	}
}

func TestGitCloneHandlerRegistration(t *testing.T) {
	r := NewRegistry()
	RegisterGitClone(r)

	factory := r.Lookup(GitCloneConfigID)
	if factory == nil {
		t.Fatal("git clone factory not found in registry")
	}
	if GitCloneConfigID == "" {
		t.Fatal("GitCloneConfigID is empty")
	}
	if GitCloneConfigID != "forge/lib/git/clone" {
		t.Fatalf("unexpected config ID: %s", GitCloneConfigID)
	}
}

func TestGitCloneInvalidConfig(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterGitClone(r)

	// Empty config should fail validation (missing object_key).
	_, err := r.CreateHandler(ctx, nil, nil, nil, nil, GitCloneConfigID, nil)
	if err == nil {
		t.Fatal("expected validation error for empty config")
	}
}

func TestResolveAuthWithoutBus(t *testing.T) {
	// Nil auth opts returns nil.
	auth, err := resolveAuthWithoutBus(nil)
	if err != nil {
		t.Fatalf("nil opts: %v", err)
	}
	if auth != nil {
		t.Fatal("expected nil auth for nil opts")
	}

	// Peer-ID-based auth returns an error.
	peerAuth := &git_block.AuthOpts{PeerId: "QmSomeTestPeerID"}
	_, err = resolveAuthWithoutBus(peerAuth)
	if err == nil {
		t.Fatal("expected error for peer-ID auth")
	}

	// Username-only returns SSH PublicKeys with the user set.
	userAuth := &git_block.AuthOpts{Username: "git"}
	auth, err = resolveAuthWithoutBus(userAuth)
	if err != nil {
		t.Fatalf("username auth: %v", err)
	}
	if auth == nil {
		t.Fatal("expected non-nil auth for username")
	}
}

func TestDefaultRegistryAndBridgeFactories(t *testing.T) {
	r := NewDefaultRegistry()

	// All built-in handlers should be registered.
	if r.Lookup(NoopConfigID) == nil {
		t.Fatal("noop not in default registry")
	}
	if r.Lookup(KvtxConfigID) == nil {
		t.Fatal("kvtx not in default registry")
	}
	if r.Lookup(GitCloneConfigID) == nil {
		t.Fatal("git/clone not in default registry")
	}

	// BridgeFactories returns a factory for every handler in the registry.
	factories := BridgeFactories(r)
	registeredIDs := r.ConfigIDs()
	if len(factories) != len(registeredIDs) {
		t.Fatalf("expected %d bridge factories (one per handler), got %d", len(registeredIDs), len(factories))
	}

	ids := map[string]bool{}
	for _, f := range factories {
		ids[f.GetConfigID()] = true
	}
	for _, id := range registeredIDs {
		if !ids[id] {
			t.Fatalf("missing bridge factory for %s", id)
		}
	}
}

func TestBridgeFactoryConstruct(t *testing.T) {
	r := NewDefaultRegistry()
	factories := BridgeFactories(r)

	// Find the noop bridge factory and verify SpaceExecConfig.
	var noopBridge *BridgeFactory
	for _, f := range factories {
		if f.GetConfigID() == NoopConfigID {
			noopBridge = f.(*BridgeFactory)
			break
		}
	}
	if noopBridge == nil {
		t.Fatal("noop bridge factory not found")
	}

	conf := noopBridge.ConstructConfig()
	if conf == nil {
		t.Fatal("ConstructConfig returned nil")
	}
	if conf.GetConfigID() != NoopConfigID {
		t.Fatalf("config ID mismatch: %s", conf.GetConfigID())
	}

	// ConstructConfig returns a SpaceExecConfig.
	sec, ok := conf.(*SpaceExecConfig)
	if !ok {
		t.Fatalf("expected *SpaceExecConfig, got %T", conf)
	}

	// UnmarshalJSON stores raw bytes.
	testJSON := []byte(`{"key":"value"}`)
	if err := sec.UnmarshalJSON(testJSON); err != nil {
		t.Fatal(err)
	}
	data, err := sec.MarshalBlock()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(testJSON) {
		t.Fatalf("expected %q, got %q", testJSON, data)
	}
}

func TestSpaceExecConfigRoundTrip(t *testing.T) {
	sec := NewSpaceExecConfig("test/handler")
	if sec.GetConfigID() != "test/handler" {
		t.Fatalf("config ID: %s", sec.GetConfigID())
	}

	// Proto roundtrip.
	protoData := []byte{0x0a, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
	if err := sec.UnmarshalVT(protoData); err != nil {
		t.Fatal(err)
	}
	out, err := sec.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(protoData) {
		t.Fatal("proto roundtrip mismatch")
	}

	// JSON roundtrip.
	jsonData := []byte(`{"foo":"bar"}`)
	if err := sec.UnmarshalJSON(jsonData); err != nil {
		t.Fatal(err)
	}
	out, err = sec.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(jsonData) {
		t.Fatal("json roundtrip mismatch")
	}

	// Block roundtrip.
	blockData := []byte("some-block-data")
	if err := sec.UnmarshalBlock(blockData); err != nil {
		t.Fatal(err)
	}
	out, err = sec.MarshalBlock()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(blockData) {
		t.Fatal("block roundtrip mismatch")
	}

	// Validate is always nil.
	if err := sec.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestUnixfsReadRegistration(t *testing.T) {
	r := NewRegistry()
	RegisterUnixfsRead(r)

	factory := r.Lookup(UnixfsReadConfigID)
	if factory == nil {
		t.Fatal("unixfs-read factory not found in registry")
	}
	if UnixfsReadConfigID != "space-exec/unixfs-read" {
		t.Fatalf("unexpected config ID: %s", UnixfsReadConfigID)
	}
}

func TestUnixfsReadEmptyConfig(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterUnixfsRead(r)

	// Empty config should fail.
	_, err := r.CreateHandler(ctx, nil, nil, nil, nil, UnixfsReadConfigID, nil)
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestUnixfsReadInvalidJSON(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterUnixfsRead(r)

	_, err := r.CreateHandler(ctx, nil, nil, nil, nil, UnixfsReadConfigID, []byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestUnixfsReadMissingObjectKey(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterUnixfsRead(r)

	_, err := r.CreateHandler(ctx, nil, nil, nil, nil, UnixfsReadConfigID, []byte(`{"file_path":"test.txt"}`))
	if err == nil {
		t.Fatal("expected error for missing object_key")
	}
}

func TestUnixfsReadValidConfig(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterUnixfsRead(r)

	// Valid config creates a handler (Execute would fail without world state).
	handler, err := r.CreateHandler(ctx, nil, nil, nil, nil, UnixfsReadConfigID, []byte(`{"object_key":"fs/test","file_path":"hello.txt"}`))
	if err != nil {
		t.Fatalf("CreateHandler: %v", err)
	}
	if handler == nil {
		t.Fatal("handler is nil")
	}
}

func TestFileHashRegistration(t *testing.T) {
	r := NewRegistry()
	RegisterFileHash(r)

	factory := r.Lookup(FileHashConfigID)
	if factory == nil {
		t.Fatal("file-hash factory not found in registry")
	}
	if FileHashConfigID != "space-exec/file-hash" {
		t.Fatalf("unexpected config ID: %s", FileHashConfigID)
	}
}

func TestFileHashEmptyConfig(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterFileHash(r)

	_, err := r.CreateHandler(ctx, nil, nil, nil, nil, FileHashConfigID, nil)
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestFileHashValidConfig(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterFileHash(r)

	handler, err := r.CreateHandler(ctx, nil, nil, nil, nil, FileHashConfigID, []byte(`{"object_key":"fs/data","file_path":"doc.md"}`))
	if err != nil {
		t.Fatalf("CreateHandler: %v", err)
	}
	if handler == nil {
		t.Fatal("handler is nil")
	}
}

func TestExportZipRegistration(t *testing.T) {
	r := NewRegistry()
	RegisterExportZip(r)

	factory := r.Lookup(ExportZipConfigID)
	if factory == nil {
		t.Fatal("export-zip factory not found in registry")
	}
	if ExportZipConfigID != "space-exec/export-zip" {
		t.Fatalf("unexpected config ID: %s", ExportZipConfigID)
	}
}

func TestExportZipEmptyConfig(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterExportZip(r)

	_, err := r.CreateHandler(ctx, nil, nil, nil, nil, ExportZipConfigID, nil)
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestExportZipInvalidJSON(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterExportZip(r)

	_, err := r.CreateHandler(ctx, nil, nil, nil, nil, ExportZipConfigID, []byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestExportZipMissingObjectKey(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterExportZip(r)

	_, err := r.CreateHandler(ctx, nil, nil, nil, nil, ExportZipConfigID, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for missing object_key")
	}
}

func TestExportZipValidConfig(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterExportZip(r)

	handler, err := r.CreateHandler(ctx, nil, nil, nil, nil, ExportZipConfigID, []byte(`{"object_key":"fs/test"}`))
	if err != nil {
		t.Fatalf("CreateHandler: %v", err)
	}
	if handler == nil {
		t.Fatal("handler is nil")
	}
}

func TestDefaultRegistryIncludesUtilityHandlers(t *testing.T) {
	r := NewDefaultRegistry()

	if r.Lookup(UnixfsReadConfigID) == nil {
		t.Fatal("unixfs-read not in default registry")
	}
	if r.Lookup(FileHashConfigID) == nil {
		t.Fatal("file-hash not in default registry")
	}
	if r.Lookup(ExportZipConfigID) == nil {
		t.Fatal("export-zip not in default registry")
	}
}

func TestNoopBridgeFullLifecycle(t *testing.T) {
	// Verify the full noop handler lifecycle through the bridge:
	// registry -> CreateHandler -> Execute -> completion.
	// Confirms no bus.Bus in the code path.
	ctx := context.Background()
	r := NewDefaultRegistry()

	handler, err := r.CreateHandler(ctx, nil, nil, nil, nil, NoopConfigID, nil)
	if err != nil {
		t.Fatalf("CreateHandler: %v", err)
	}
	if err := handler.Execute(ctx); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}
