package bldr_project_controller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/db/bucket"
)

func TestManifestStartupBuildStatePath(t *testing.T) {
	conf := NewManifestBuilderConfig("demo", "dev", "desktop/linux/amd64", "devtool")
	meta := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 7)
	result := bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: "manifest-bucket"},
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
	)
	state := NewManifestStartupBuildState(conf, result)

	if err := state.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	path, err := state.GetStatePath("/tmp/bldr")
	if err != nil {
		t.Fatalf("get state path: %v", err)
	}
	expected := filepath.Join(
		"/tmp/bldr",
		"cache",
		manifestStartupBuildStateDirName,
		"demo",
		"dev",
		"desktop",
		"linux",
		"amd64",
		getManifestStartupBuildStateFileName(conf),
	)
	if path != expected {
		t.Fatalf("expected %q, got %q", expected, path)
	}
}

func TestManifestStartupBuildStateValidateMetaMismatch(t *testing.T) {
	conf := NewManifestBuilderConfig("demo", "dev", "desktop/linux/amd64", "devtool")
	meta := bldr_manifest.NewManifestMeta("other", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1)
	result := bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: "manifest-bucket"},
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
	)
	state := NewManifestStartupBuildState(conf, result)

	if err := state.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestManifestStartupBuildStateWriteFile(t *testing.T) {
	conf := NewManifestBuilderConfig("demo", "dev", "desktop/linux/amd64", "devtool")
	meta := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 7)
	result := bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: "manifest-bucket"},
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
	)
	state := NewManifestStartupBuildState(conf, result)

	tmpDir := t.TempDir()
	if err := state.WriteFile(tmpDir); err != nil {
		t.Fatalf("write file: %v", err)
	}
	statePath, err := state.GetStatePath(tmpDir)
	if err != nil {
		t.Fatalf("get state path: %v", err)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state path: %v", err)
	}
	decoded := &ManifestStartupBuildState{}
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.EqualVT(state) {
		t.Fatal("decoded state did not match written state")
	}
}

func TestReadManifestStartupBuildStateMissing(t *testing.T) {
	conf := NewManifestBuilderConfig("demo", "dev", "desktop/linux/amd64", "devtool")

	state, err := ReadManifestStartupBuildState(t.TempDir(), conf)
	if err != nil {
		t.Fatalf("read missing state: %v", err)
	}
	if state != nil {
		t.Fatal("expected nil state")
	}
}

func TestReadManifestStartupBuildStateCorrupt(t *testing.T) {
	conf := NewManifestBuilderConfig("demo", "dev", "desktop/linux/amd64", "devtool")
	tmpDir := t.TempDir()
	statePath, err := getManifestStartupBuildStatePath(tmpDir, conf)
	if err != nil {
		t.Fatalf("get state path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir all: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("bad-data"), 0o644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	state, err := ReadManifestStartupBuildState(tmpDir, conf)
	if err == nil {
		t.Fatalf("expected validation error, got state %#v", state)
	}
}

func TestRemoveManifestStartupBuildState(t *testing.T) {
	conf := NewManifestBuilderConfig("demo", "dev", "desktop/linux/amd64", "devtool")
	meta := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 7)
	result := bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: "manifest-bucket"},
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
	)
	state := NewManifestStartupBuildState(conf, result)

	tmpDir := t.TempDir()
	if err := state.WriteFile(tmpDir); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := RemoveManifestStartupBuildState(tmpDir, conf); err != nil {
		t.Fatalf("remove state: %v", err)
	}
	loaded, err := ReadManifestStartupBuildState(tmpDir, conf)
	if err != nil {
		t.Fatalf("read removed state: %v", err)
	}
	if loaded != nil {
		t.Fatal("expected removed state to be nil")
	}
}

func TestManifestStartupBuildStateWriteFileLongOverride(t *testing.T) {
	conf := NewManifestBuilderConfig("demo", "release", "desktop/darwin/arm64", "devtool")
	conf.BuilderConfigOverride = &configset_proto.ControllerConfig{
		Id:     "bldr/dist/compiler",
		Config: []byte(strings.Repeat("x", 4096)),
	}
	meta := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_RELEASE, "desktop/darwin/arm64", 7)
	result := bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: "manifest-bucket"},
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
	)
	state := NewManifestStartupBuildState(conf, result)

	tmpDir := t.TempDir()
	if err := state.WriteFile(tmpDir); err != nil {
		t.Fatalf("write file with long override: %v", err)
	}
	statePath, err := state.GetStatePath(tmpDir)
	if err != nil {
		t.Fatalf("get state path: %v", err)
	}
	if got := len(filepath.Base(statePath)); got >= 255 {
		t.Fatalf("expected short cache filename, got length %d", got)
	}
}
