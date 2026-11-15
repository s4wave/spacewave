package world

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/net/peer"
)

// standInWorldState is a WorldState whose ApplyWorldOp runs apply instead of
// an engine; its other methods are unused.
type standInWorldState struct {
	WorldState
	apply func(ctx context.Context) error
}

// ApplyWorldOp runs apply.
func (s *standInWorldState) ApplyWorldOp(ctx context.Context, op Operation, opSender peer.ID) (uint64, bool, error) {
	if err := s.apply(ctx); err != nil {
		return 0, false, err
	}
	return 1, false, nil
}

// standInOp is an op that only names its type.
type standInOp struct{ Operation }

// TypeID returns the op's type.
func (standInOp) TypeID() string { return "test/stand-in" }

// keyGuard refuses writes to its key with refusal.
type keyGuard struct {
	key     string
	refusal error
}

// CheckWrite refuses a write naming the guard's key.
func (g *keyGuard) CheckWrite(ctx context.Context, ws WorldState, w *GuardedWrite) error {
	for _, key := range w.ObjectKeys {
		if key == g.key {
			return g.refusal
		}
	}
	return nil
}

// TestGuardedApplyFailsClosed verifies that a guarded apply fails and discards
// its transaction when the engine ignores the guard, when the op swallows a
// guard error, and when the op fails on its own.
func TestGuardedApplyFailsClosed(t *testing.T) {
	refusal := &OperationRejection{Message: "refused"}
	readErr := errors.New("read failed")
	opErr := errors.New("op failed")
	// createSwallowed is an engine that guards the op, which swallows the
	// error of creating key "protected".
	createSwallowed := func(ctx context.Context) error {
		ws := GuardOpWorldState(ctx, nil)
		obj, _ := ws.CreateObject(ctx, "protected", nil)
		ReleaseObjectState(obj)
		return nil
	}
	for _, tc := range []struct {
		name       string
		refusal    error
		apply      func(ctx context.Context) error
		wantErr    error
		wantSysErr bool
		wantDrop   bool
	}{
		{"engine ignores the guard", refusal, func(context.Context) error { return nil }, ErrEngineUnguardedOp, false, true},
		{"op swallows a refusal", refusal, createSwallowed, refusal, false, true},
		{"op swallows a guard read error", readErr, createSwallowed, readErr, true, true},
		{"op fails on its own", refusal, func(ctx context.Context) error {
			GuardOpWorldState(ctx, nil)
			return opErr
		}, opErr, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dropped := false
			ws := &guardedWorldState{
				ws:       &standInWorldState{apply: tc.apply},
				guard:    &keyGuard{key: "protected", refusal: tc.refusal},
				opFailed: func() { dropped = true },
			}
			_, sysErr, err := ws.ApplyWorldOp(context.Background(), standInOp{}, "")
			if !errors.Is(err, tc.wantErr) || sysErr != tc.wantSysErr {
				t.Fatalf("apply returned sysErr %v, %v; want %v, %v", sysErr, err, tc.wantSysErr, tc.wantErr)
			}
			if dropped != tc.wantDrop {
				t.Fatalf("transaction discarded %v, want %v", dropped, tc.wantDrop)
			}
		})
	}
}
