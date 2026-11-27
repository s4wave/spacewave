package world

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/peer"
)

// standInWorldState is a WorldState whose ApplyWorldOp runs apply instead of
// an engine. Its other methods are unused.
type standInWorldState struct {
	WorldState
	apply func(ctx context.Context) error
	// written are the keys CreateObject wrote.
	written []string
}

// ApplyWorldOp runs apply.
func (s *standInWorldState) ApplyWorldOp(ctx context.Context, op Operation, opSender peer.ID) (uint64, bool, error) {
	if err := s.apply(ctx); err != nil {
		return 0, false, err
	}
	return 1, false, nil
}

// CreateObject records the key as written.
func (s *standInWorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (ObjectState, error) {
	s.written = append(s.written, key)
	return nil, nil
}

// standInOp is an op that only reports its type.
type standInOp struct{ Operation }

// TypeID returns the op's type.
func (standInOp) TypeID() string { return "test/stand-in" }

// keyGuard refuses writes to its key with refusal.
type keyGuard struct {
	key     string
	refusal error
}

// CheckWrite refuses a write to the guard's key.
func (g *keyGuard) CheckWrite(ctx context.Context, ws WorldState, w *GuardedWrite) error {
	if slices.Contains(w.ObjectKeys, g.key) {
		return g.refusal
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
		{"op swallows a nested op no engine guarded", refusal, func(ctx context.Context) error {
			GuardOpWorldState(ctx, nil)
			nested := &guardedWorldState{ws: &standInWorldState{apply: func(context.Context) error { return nil }}, guard: &keyGuard{key: "protected", refusal: refusal}}
			_, _, _ = nested.ApplyWorldOp(ctx, standInOp{}, "")
			return nil
		}, ErrEngineUnguardedOp, false, true},
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

// passWorldState is a WorldState wrapper that is not a guarded layer: every
// method, ApplyWorldOp included, passes to the state it wraps.
type passWorldState struct{ WorldState }

// TestStackedGuardsFold verifies that a nested op applied through guards
// stacked on one engine state is checked by every guard in one apply the
// engine takes: a write either guard refuses fails the nested op, and the op
// whose handle it went through, even when the nested op swallows the
// refusal, and nothing reaches the engine. A write both allow succeeds, and a
// failure discards the transaction once. It stacks the outer guard on an op's
// handle, holds both in one op's handle, stacks two guarded transactions, and
// puts a wrapper the fold does not see between the guard and a handle, where
// both guards still check each write and an allowed op fails closed.
func TestStackedGuardsFold(t *testing.T) {
	refusal := &OperationRejection{Message: "refused"}
	allow := &keyGuard{key: "other", refusal: refusal}
	shapes := []struct {
		name string
		// stack returns the state the nested op is applied through, and the
		// op whose handle it is, or nil.
		stack func(engine WorldState, outer, inner WriteGuard, discard func()) (WorldState, *opApply)
		// unseen is set when the fold does not see the inner layer.
		unseen bool
	}{
		{"guard over an op's handle", func(engine WorldState, outer, inner WriteGuard, discard func()) (WorldState, *opApply) {
			op := newOpApply(nil, []WriteGuard{inner})
			op.taken.Store(true)
			return GuardWorldState(&guardedWorldState{ws: engine, guard: inner, apply: op}, outer), op
		}, false},
		{"op handle with two guards", func(engine WorldState, outer, inner WriteGuard, discard func()) (WorldState, *opApply) {
			op := newOpApply(nil, []WriteGuard{inner, outer})
			return GuardOpWorldState(context.WithValue(context.Background(), opApplyKey{}, op), engine), op
		}, false},
		{"guarded transaction over a guarded transaction", func(engine WorldState, outer, inner WriteGuard, discard func()) (WorldState, *opApply) {
			innerTx := &guardedTx{guardedWorldState: &guardedWorldState{ws: engine, guard: inner, opFailed: discard}}
			return &guardedTx{guardedWorldState: &guardedWorldState{ws: innerTx, guard: outer, opFailed: discard}}, nil
		}, false},
		{"guard over a wrapper over an op's handle", func(engine WorldState, outer, inner WriteGuard, discard func()) (WorldState, *opApply) {
			op := newOpApply(nil, []WriteGuard{inner})
			op.taken.Store(true)
			return GuardWorldState(&passWorldState{&guardedWorldState{ws: engine, guard: inner, apply: op}}, outer), op
		}, true},
	}
	cases := []struct {
		name         string
		outer, inner WriteGuard
		guarded      bool
		refused      bool
	}{
		{"outer guard refuses", &keyGuard{key: "protected", refusal: refusal}, allow, true, true},
		{"inner guard refuses", allow, &keyGuard{key: "protected", refusal: refusal}, true, true},
		{"both guards allow", allow, &keyGuard{key: "other", refusal: refusal}, true, false},
		{"engine ignores the guards", allow, &keyGuard{key: "other", refusal: refusal}, false, false},
	}
	for _, shape := range shapes {
		for _, tc := range cases {
			t.Run(shape.name+"/"+tc.name, func(t *testing.T) {
				// engine applies the nested op, which writes "protected" and
				// swallows the error, guarded when tc.guarded.
				engine := &standInWorldState{}
				engine.apply = func(ctx context.Context) error {
					if !tc.guarded {
						return nil
					}
					obj, _ := GuardOpWorldState(ctx, engine).CreateObject(ctx, "protected", nil)
					ReleaseObjectState(obj)
					return nil
				}
				discards := 0
				stacked, op := shape.stack(engine, tc.outer, tc.inner, func() { discards++ })

				var wantErr error
				switch {
				case tc.refused:
					wantErr = refusal
				case !tc.guarded || shape.unseen:
					wantErr = ErrEngineUnguardedOp
				}
				_, _, err := stacked.ApplyWorldOp(context.Background(), standInOp{}, "")
				if !errors.Is(err, wantErr) || (wantErr == nil) != (err == nil) {
					t.Fatalf("apply returned %v, want %v", err, wantErr)
				}
				wantWritten := tc.guarded && !tc.refused
				if wrote := slices.Contains(engine.written, "protected"); wrote != wantWritten {
					t.Fatalf("engine wrote %v, want %v", wrote, wantWritten)
				}
				if tc.refused && op != nil && !errors.Is(op.guardErr(), refusal) {
					t.Fatalf("outer op recorded %v, want the refusal", op.guardErr())
				}
				if op == nil && discards != boolInt(err != nil) {
					t.Fatalf("transaction discarded %d times", discards)
				}
			})
		}
	}
}

// boolInt returns 1 for true and 0 for false.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// standInObjectState is an ObjectState whose ApplyObjectOp runs apply. It
// records IncrementRev.
type standInObjectState struct {
	ObjectState
	apply func(ctx context.Context) error
	// bumped is set when IncrementRev ran.
	bumped bool
}

// GetKey returns the object's key.
func (o *standInObjectState) GetKey() string { return "object" }

// ApplyObjectOp runs apply.
func (o *standInObjectState) ApplyObjectOp(ctx context.Context, op Operation, opSender peer.ID) (uint64, bool, error) {
	if err := o.apply(ctx); err != nil {
		return 0, false, err
	}
	return 1, false, nil
}

// IncrementRev records the write.
func (o *standInObjectState) IncrementRev(ctx context.Context) (uint64, error) {
	o.bumped = true
	return 1, nil
}

// writeGuard refuses every write made by an op, while allowing the op's type
// check, with refusal when refuse is set.
type writeGuard struct {
	refuse  bool
	refusal error
}

// CheckWrite refuses a write that has no op type.
func (g *writeGuard) CheckWrite(ctx context.Context, ws WorldState, w *GuardedWrite) error {
	if g.refuse && w.OpTypeID == "" {
		return g.refusal
	}
	return nil
}

// TestStackedObjectGuardsFold verifies that an object op applied through a
// guard stacked on an op's object handle is checked by both guards in one
// apply the engine takes, as TestStackedGuardsFold does for World ops: the
// object write either guard refuses fails the op and the op whose handle it
// went through, and a write both allow reaches the engine.
func TestStackedObjectGuardsFold(t *testing.T) {
	refusal := &OperationRejection{Message: "refused"}
	refuse := func() WriteGuard { return &writeGuard{refuse: true, refusal: refusal} }
	allow := func() WriteGuard { return &writeGuard{refusal: refusal} }
	for _, tc := range []struct {
		name         string
		outer, inner WriteGuard
		refused      bool
	}{
		{"outer guard refuses", refuse(), allow(), true},
		{"inner guard refuses", allow(), refuse(), true},
		{"both guards allow", allow(), allow(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The engine applies the object op, which bumps the object's
			// revision through its guarded handle and swallows the error.
			obj := &standInObjectState{}
			obj.apply = func(ctx context.Context) error {
				_, _ = GuardOpObjectState(ctx, obj, nil).IncrementRev(ctx)
				return nil
			}
			op := newOpApply(nil, []WriteGuard{tc.inner})
			op.taken.Store(true)
			handle := &guardedObjectState{obj: obj, guard: tc.inner, apply: op}
			stacked := GuardObjectState(handle, nil, tc.outer)
			defer ReleaseObjectState(stacked)

			_, _, err := stacked.ApplyObjectOp(context.Background(), standInOp{}, "")
			if tc.refused != errors.Is(err, refusal) || (!tc.refused && err != nil) {
				t.Fatalf("apply returned %v, want refused %v", err, tc.refused)
			}
			if obj.bumped == tc.refused {
				t.Fatalf("engine wrote %v, want %v", obj.bumped, !tc.refused)
			}
			if tc.refused && !errors.Is(op.guardErr(), refusal) {
				t.Fatalf("outer op recorded %v, want the refusal", op.guardErr())
			}
		})
	}
}
