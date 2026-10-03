//go:build !js && !wasip1

package bolt

import (
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/coord"
)

// TestLeaseRefreshRetainsReaders verifies that post-commit generation refresh
// does not remap storage beneath snapshots opened during the same lease.
func TestLeaseRefreshRetainsReaders(t *testing.T) {
	// Open a coordinator for the reader-retention lease.
	ctx := t.Context()
	db := openTestDB(t)
	c := NewCoordinator(db, nil)

	// Acquire the write lease and retain it through the reader check.
	lease, err := c.WaitAcquireWriteLease(ctx, coord.Scope{VolumeID: "volume", ObjectStoreID: "objects"})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release(ctx)

	// Refresh the lease before committing and opening its snapshot reader.
	if _, err := lease.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	writeBoltValue(t, db, "published")

	// Open a read transaction on the committed database.
	reader, err := db.Begin(false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Rollback()

	// Refresh the lease while its read transaction remains open.
	done := make(chan error, 1)
	go func() {
		snapshot, err := lease.Refresh(ctx)
		if err == nil && snapshot.Generation != 1 {
			t.Errorf("generation = %d, want 1", snapshot.Generation)
		}
		done <- err
	}()

	// Verify the refresh completes while the read transaction remains open.
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		_ = reader.Rollback()
		<-done
		t.Fatal("refresh waited for a reader opened within the held lease")
	}
}
