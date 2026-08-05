//go:build !js

package goscriptbench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEngineCapabilityRoundTrip(t *testing.T) {
	capability := validEngineCapability()
	dir, err := PublishEngineCapability(t.TempDir(), capability)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadEngineCapability(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != capability {
		t.Fatalf("capability = %#v, want %#v", got, capability)
	}
}

func TestEngineCapabilityRejectsIncompleteAndUnexpectedRecords(t *testing.T) {
	invalid := validEngineCapability()
	invalid.Reason = ""
	if _, err := PublishEngineCapability(t.TempDir(), invalid); err == nil {
		t.Fatal("expected missing reason to fail")
	}

	root := t.TempDir()
	dir, err := PublishEngineCapability(root, validEngineCapability())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEngineCapability(dir); err == nil {
		t.Fatal("expected unexpected capability file to fail")
	}
}

func validEngineCapability() EngineCapability {
	return EngineCapability{
		SchemaVersion: engineCapabilitySchemaVersion,
		RunID:         "run-1",
		Engine:        "webkit",
		EngineVersion: "26.5",
		Capability:    engineCapabilityOPFS,
		Status:        engineCapabilityUnsupported,
		Reason:        "navigator.storage.getDirectory is unavailable",
	}
}
