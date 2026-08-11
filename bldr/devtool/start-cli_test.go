//go:build !js

package devtool

import (
	"context"
	"testing"
)

func TestWaitForNewManifestRevisionDetectsFirstSnapshot(t *testing.T) {
	waits := 0
	err := waitForNewManifestRevision(
		context.Background(),
		41,
		func(context.Context) (uint64, uint64, error) { return 9, 42, nil },
		func(context.Context, uint64) error { waits++; return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if waits != 0 {
		t.Fatalf("waits = %d, want 0 for N to N+1 startup race", waits)
	}
}

func TestWaitForNewManifestRevisionWaitsFromCollectedSequence(t *testing.T) {
	revisions := []uint64{41, 42}
	var snapshotCalls int
	var waited []uint64
	err := waitForNewManifestRevision(
		context.Background(),
		41,
		func(context.Context) (uint64, uint64, error) {
			rev := revisions[snapshotCalls]
			snapshotCalls++
			return uint64(10 + snapshotCalls), rev, nil
		},
		func(_ context.Context, seqno uint64) error { waited = append(waited, seqno); return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(waited) != 1 || waited[0] != 11 {
		t.Fatalf("waited sequences = %v, want [11]", waited)
	}
}
