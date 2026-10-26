// Package s4wave_alarm is the contract of Alarms: objects that flag a
// condition while it holds and resolve when it clears.
package s4wave_alarm

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// ErrNotFiring is returned when acknowledging an Alarm that is not firing.
var ErrNotFiring = errors.New("the alarm is not firing")

// Condition is one evaluation of the condition an Alarm flags.
type Condition struct {
	// Firing is set while the condition holds.
	Firing bool
	// Summary describes the condition.
	Summary string
	// Options are the actions that would change the condition.
	Options []*AlarmOption
}

// IsSounding reports whether the Alarm is firing, acknowledged or not.
func (a *Alarm) IsSounding() bool {
	switch a.GetState() {
	case AlarmState_ALARM_STATE_FIRING, AlarmState_ALARM_STATE_ACKNOWLEDGED:
		return true
	}
	return false
}

// Next returns the Alarm after cond was found at now, with the condition
// holding for hold before the Alarm fires. A nil Alarm starts clear. The
// caller writes the result only when it differs from a.
func (a *Alarm) Next(cond *Condition, hold time.Duration, now time.Time) *Alarm {
	// Start from the current Alarm with the condition's description.
	next := a.CloneVT()
	if next == nil {
		next = &Alarm{}
	}
	nowMs := now.UnixMilli()
	next.HoldMs = max(hold, 0).Milliseconds()
	next.Summary = cond.Summary
	next.Options = cond.Options
	next.Error = ""

	// A condition that does not hold resolves the Alarm from any state.
	if !cond.Firing {
		if next.GetState() != AlarmState_ALARM_STATE_CLEAR {
			next.State = AlarmState_ALARM_STATE_CLEAR
			next.ClearedMs = nowMs
		}
		next.SinceMs = 0
		next.FiredMs = 0
		next.AcknowledgedMs = 0
		next.AcknowledgedBy = ""
		return next
	}

	// A condition that holds starts the hold time, then fires the Alarm once
	// it has held that long. A sounding Alarm stays as it is.
	if next.GetState() == AlarmState_ALARM_STATE_CLEAR {
		next.State = AlarmState_ALARM_STATE_HOLDING
		next.SinceMs = nowMs
	}
	if next.GetState() == AlarmState_ALARM_STATE_HOLDING && nowMs-next.GetSinceMs() >= next.GetHoldMs() {
		next.State = AlarmState_ALARM_STATE_FIRING
		next.FiredMs = nowMs
	}
	return next
}

// HoldEnds returns when a holding Alarm fires if its condition still holds,
// and false for an Alarm that is not holding.
func (a *Alarm) HoldEnds() (time.Time, bool) {
	if a.GetState() != AlarmState_ALARM_STATE_HOLDING {
		return time.Time{}, false
	}
	return time.UnixMilli(a.GetSinceMs() + a.GetHoldMs()), true
}

// Acknowledge records that by acknowledged the firing Alarm at now. The Alarm
// keeps sounding until its condition clears.
func (a *Alarm) Acknowledge(by string, now time.Time) error {
	// Only a firing Alarm takes an acknowledgement.
	if a.GetState() != AlarmState_ALARM_STATE_FIRING {
		return ErrNotFiring
	}

	// Record who acknowledged it and when.
	a.State = AlarmState_ALARM_STATE_ACKNOWLEDGED
	a.AcknowledgedMs = now.UnixMilli()
	a.AcknowledgedBy = by
	return nil
}

// CreateAlarm creates the World object key as an alarm holding a.
func CreateAlarm(ctx context.Context, ws world.WorldState, key string, a *Alarm) error {
	// Create the object holding the Alarm.
	obj, _, err := world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(a, true)
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		return errors.Wrapf(err, "create alarm %q", key)
	}

	// Mark it as an alarm.
	return world_types.SetObjectType(ctx, ws, key, AlarmTypeID)
}

// ReadAlarm reads the Alarm key, or returns nil when the object is missing.
func ReadAlarm(ctx context.Context, ws world.WorldState, key string) (*Alarm, error) {
	// Look up the Alarm's object.
	obj, found, err := ws.GetObject(ctx, key)
	if err != nil || !found {
		world.ReleaseObjectState(obj)
		return nil, err
	}
	defer world.ReleaseObjectState(obj)

	// Decode the Alarm from the object's root block.
	var a *Alarm
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		a, err = block.UnmarshalBlock[Alarm](ctx, bcs)
		return err
	})
	if err != nil {
		return nil, errors.Wrapf(err, "read alarm %q", key)
	}
	if a == nil {
		a = &Alarm{}
	}
	return a, nil
}

// WriteAlarm stores a as the Alarm key, creating the object when it is
// missing.
func WriteAlarm(ctx context.Context, ws world.WorldState, key string, a *Alarm) error {
	// Create a missing Alarm.
	obj, found, err := ws.GetObject(ctx, key)
	if err != nil || !found {
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		return CreateAlarm(ctx, ws, key, a)
	}
	defer world.ReleaseObjectState(obj)

	// Replace the Alarm's root block.
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(a, true)
		return nil
	})
	return errors.Wrapf(err, "write alarm %q", key)
}

// AcknowledgeAlarm acknowledges the firing Alarm key as by at now. Run it
// inside a write transaction so the read and the write see the same Alarm.
func AcknowledgeAlarm(ctx context.Context, ws world.WorldState, key, by string, now time.Time) error {
	// Read the Alarm.
	a, err := ReadAlarm(ctx, ws, key)
	if err != nil {
		return err
	}
	if a == nil {
		return errors.Wrapf(world.ErrObjectNotFound, "alarm %q", key)
	}

	// Acknowledge it and store the result.
	if err := a.Acknowledge(by, now); err != nil {
		return err
	}
	return WriteAlarm(ctx, ws, key, a)
}
