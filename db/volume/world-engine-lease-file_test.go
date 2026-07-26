//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly || solaris || windows

package volume

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestFileWorldEngineLeaseNamespacesBackingStore(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "volume-a.db")
	pathB := filepath.Join(dir, "volume-b.db")
	providerA := NewFileWorldEngineLeaseProvider(dir, pathA)
	providerB := NewFileWorldEngineLeaseProvider(dir, pathB)

	leaseA, err := providerA.AcquireWorldEngineLease(context.Background(), "shared-object")
	if err != nil {
		t.Fatalf("acquire lease for volume A: %v", err)
	}
	t.Cleanup(func() { _ = leaseA.Release() })

	leaseB, err := providerB.AcquireWorldEngineLease(context.Background(), "shared-object")
	if err != nil {
		t.Fatalf("acquire lease for volume B: %v", err)
	}
	t.Cleanup(func() { _ = leaseB.Release() })
}
func TestFileWorldEngineLeaseCanonicalizesBackingStorePath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	relativePath := "volume.db"
	absolutePath := filepath.Join(dir, relativePath)
	providerA := NewFileWorldEngineLeaseProvider(dir, relativePath)
	providerB := NewFileWorldEngineLeaseProvider(dir, absolutePath)

	lease, err := providerA.AcquireWorldEngineLease(context.Background(), "shared-object")
	if err != nil {
		t.Fatalf("acquire relative-path lease: %v", err)
	}
	t.Cleanup(func() { _ = lease.Release() })

	_, err = providerB.AcquireWorldEngineLease(context.Background(), "shared-object")
	var heldErr *WorldEngineLeaseHeldError
	if !errors.As(err, &heldErr) {
		t.Fatalf("absolute-path lease error = %v, want WorldEngineLeaseHeldError", err)
	}
}
