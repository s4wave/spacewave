package space_world

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
)

// MaxGraphSettingsStep is the most links a graph settings step indexes, and
// the most quad changes a regeneration's switch carries.
const MaxGraphSettingsStep = maxGraphSettingsStep

// ReasonRegenMoved is the refusal of a switch past the bound.
const ReasonRegenMoved = reasonRegenMoved

// StopRegenBuild stops e's off-path regeneration builds, so its replay builds
// each switch inline, as a replica that lags its builder does. The engine's
// own RegenerateWorld then waits forever; order the pin and switch with
// WriteRegenOp.
func StopRegenBuild(e *Engine) {
	e.a.regenBuild.routine.SetStateRoutine(func(context.Context, *bucket.ObjectRef) error { return nil })
}

// WriteRegenOp orders op through e and waits for its decision.
func WriteRegenOp(ctx context.Context, e *Engine, op *RegenerateWorldOp) error {
	return e.a.writeOp(ctx, &WorldOp{Body: &WorldOp_RegenerateWorld{RegenerateWorld: op}})
}

// WriteCarriedQuads sets the quads of the quad list at quads in e's World as
// RegenerateWorld does before it pins.
func WriteCarriedQuads(ctx context.Context, e *Engine, quads *block.BlockRef) error {
	return e.writeCarriedQuads(ctx, quads)
}

// RegenCarryQuads is the most quads one carried-quad write sets.
const RegenCarryQuads = regenCarryQuads

// WaitRegenBuild waits until e's installed head holds a pin and e built it
// off the replay path, and returns the refusal the build met, or "".
func WaitRegenBuild(ctx context.Context, e *Engine) (string, error) {
	res, err := e.a.regenBuild.waitPinned(ctx)
	if err != nil || res == nil {
		return "", err
	}
	return res.invalid, nil
}

// HeldRegenPin returns the pin e's installed head holds, or nil.
func HeldRegenPin(e *Engine) *block.BlockRef {
	var pin *block.BlockRef
	e.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		pin, _, _ = heldRegen(e.a.headState)
	})
	return pin
}

// ReadSavedCursor returns the replay cursor the Adapter of so saved, or nil.
func ReadSavedCursor(ctx context.Context, so sobject.SharedObject) (*ReplayCursor, error) {
	return readSavedCursor(ctx, so)
}

// DeleteSavedCursor deletes the replay cursor the Adapter of so saved.
func DeleteSavedCursor(ctx context.Context, so sobject.SharedObject) error {
	return writeLocalState(ctx, so, replayCursorStoreID, replayCursorKey, nil)
}

// WriteSavedCursor saves cursor as the replay cursor of the Adapter of so.
func WriteSavedCursor(ctx context.Context, so sobject.SharedObject, cursor *ReplayCursor) error {
	data, err := cursor.MarshalVT()
	if err != nil {
		return err
	}
	return writeLocalState(ctx, so, replayCursorStoreID, replayCursorKey, data)
}

// SetAdmittedHook makes fn see each candidate an Adapter keeps until the
// returned func clears it.
func SetAdmittedHook(fn func(world.Candidate)) func() {
	admittedHook.Store(&fn)
	return func() { admittedHook.Store(nil) }
}
