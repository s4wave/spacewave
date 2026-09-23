package provider_local

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestProcessOperationsConcurrentAcceptance replays against the new accepted
// root when another validator finishes the same batch during local processing.
func TestProcessOperationsConcurrentAcceptance(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, _, account, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	object, releaseObject, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseObject()
	id, err := object.QueueOperation(ctx, []byte("advance"))
	if err != nil {
		t.Fatal(err)
	}
	states, releaseStates, err := object.(sobject.InviteHost).GetSOHost().GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()
	if _, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return len(state.GetOps()) != 0, nil
	}, nil); err != nil {
		t.Fatal(err)
	}

	accept := func(_ context.Context, _ sobject.SharedObjectStateSnapshot, _ []byte, ops []*sobject.SOOperationInner) (*[]byte, []*sobject.SOOperationResult, error) {
		data := []byte("accepted")
		results := make([]*sobject.SOOperationResult, len(ops))
		for i, op := range ops {
			results[i] = sobject.BuildSOOperationResult(op.GetPeerId(), op.GetNonce(), true, nil)
		}
		return &data, results, nil
	}
	called := false
	err = object.ProcessOperations(ctx, false, func(ctx context.Context, snapshot sobject.SharedObjectStateSnapshot, data []byte, ops []*sobject.SOOperationInner) (*[]byte, []*sobject.SOOperationResult, error) {
		called = true
		if err := object.ProcessOperations(ctx, false, accept); err != nil {
			return nil, nil, err
		}
		return accept(ctx, snapshot, data, ops)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("pending operation was not processed")
	}
	if _, _, err := object.WaitOperation(ctx, id); err != nil {
		t.Fatal(err)
	}
}
