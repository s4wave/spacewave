package session_controller

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestDeleteSessionStopsMountedLifetime checks that deletion commits before
// cleanup, permits cleanup to call the controller, and forgets completed mounts.
func TestDeleteSessionStopsMountedLifetime(t *testing.T) {
	// Register the Session in the production controller's in-memory store.
	ctx := t.Context()
	controller := &Controller{objStore: store_kvtx_inmem.NewStore()}
	ref := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		Id: "session", ProviderId: "spacewave", ProviderAccountId: "account",
	}}
	entry, err := controller.RegisterSession(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Bind provider cleanup that reenters the registry after deletion has committed.
	stopped := false
	var release func()
	release = controller.TrackSession(ref, func(ctx context.Context) error {
		// Verify that logout committed its registry change before invoking cleanup.
		stored, err := controller.GetSessionByIdx(ctx, entry.GetSessionIndex())
		if err != nil {
			return err
		}
		if stored != nil {
			t.Fatal("provider cleanup ran before the Session registration was deleted")
		}

		// Release the provider's retained lifetime through the controller API.
		stopped = true
		release()
		return nil
	})

	// Delete through the public operation and require cleanup before it returns.
	if err := controller.DeleteSession(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("DeleteSession returned without stopping the mounted Session")
	}

	// Replaying logout must not invoke a lifetime that has already released.
	stopped = false
	if err := controller.DeleteSession(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if stopped {
		t.Fatal("DeleteSession stopped an already released lifetime")
	}
}
