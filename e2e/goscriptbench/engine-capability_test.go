//go:build !js

package goscriptbench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEngineCapabilityRoundTrip(t *testing.T) {
	// Publish a valid capability record and read it back.
	capability := validEngineCapability()
	dir, err := PublishEngineCapability(t.TempDir(), capability)
	if err != nil {
		t.Fatal(err)
	}

	// Require the readback to match the published record.
	got, err := ReadEngineCapability(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != capability {
		t.Fatalf("capability = %#v, want %#v", got, capability)
	}
}

func TestEngineCapabilityRejectsIncompleteAndUnexpectedRecords(t *testing.T) {
	// Reject a capability record missing its reason.
	invalid := validEngineCapability()
	invalid.Reason = ""
	if _, err := PublishEngineCapability(t.TempDir(), invalid); err == nil {
		t.Fatal("expected missing reason to fail")
	}

	// Publish a valid record, then corrupt it with an unexpected file.
	root := t.TempDir()
	dir, err := PublishEngineCapability(root, validEngineCapability())
	if err != nil {
		t.Fatal(err)
	}

	// Overwrite the result with an empty record.
	if err := os.WriteFile(filepath.Join(dir, "result.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Require the corrupted record to fail validation.
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
