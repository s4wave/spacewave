package s4wave_alarm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// TestAlarmNext walks an Alarm through holding, firing, acknowledged and
// clear.
func TestAlarmNext(t *testing.T) {
	// Start with no Alarm and a two minute hold.
	start := time.UnixMilli(1_793_000_000_000)
	hold := 2 * time.Minute
	down := &Condition{Firing: true, Summary: "forge is down", Options: []*AlarmOption{{Label: "Restart node_exporter", Recommended: true}}}
	up := &Condition{Summary: "every instance is up"}

	// A clear condition leaves a new Alarm clear with nothing to record.
	a := (*Alarm)(nil).Next(up, hold, start)
	if a.GetState() != AlarmState_ALARM_STATE_CLEAR || a.GetClearedMs() != 0 {
		t.Fatalf("new clear alarm: %v", a)
	}

	// A condition that holds starts the hold time.
	a = a.Next(down, hold, start)
	if a.GetState() != AlarmState_ALARM_STATE_HOLDING || a.GetSinceMs() != start.UnixMilli() {
		t.Fatalf("holding alarm: %v", a)
	}
	ends, ok := a.HoldEnds()
	if !ok || !ends.Equal(start.Add(hold)) {
		t.Fatalf("hold ends %v %v", ends, ok)
	}

	// Before the hold time passes the Alarm holds without change.
	same := a.Next(down, hold, start.Add(time.Minute))
	if !same.EqualVT(a) {
		t.Fatalf("alarm changed during its hold: %v", same)
	}

	// Once the condition has held for the hold time the Alarm fires.
	fireAt := start.Add(hold)
	a = a.Next(down, hold, fireAt)
	if a.GetState() != AlarmState_ALARM_STATE_FIRING || a.GetFiredMs() != fireAt.UnixMilli() || !a.IsSounding() {
		t.Fatalf("firing alarm: %v", a)
	}

	// Acknowledging keeps it sounding, and a later evaluation keeps the
	// acknowledgement.
	if err := a.Acknowledge("cjs", fireAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	a = a.Next(down, hold, fireAt.Add(2*time.Minute))
	if a.GetState() != AlarmState_ALARM_STATE_ACKNOWLEDGED || a.GetAcknowledgedBy() != "cjs" || !a.IsSounding() {
		t.Fatalf("acknowledged alarm: %v", a)
	}
	if err := a.Acknowledge("cjs", fireAt); !errors.Is(err, ErrNotFiring) {
		t.Fatalf("second acknowledge returned %v", err)
	}

	// The condition clearing resolves the Alarm and forgets the episode.
	clearAt := fireAt.Add(5 * time.Minute)
	a = a.Next(up, hold, clearAt)
	if a.GetState() != AlarmState_ALARM_STATE_CLEAR || a.GetClearedMs() != clearAt.UnixMilli() {
		t.Fatalf("cleared alarm: %v", a)
	}
	if a.GetSinceMs() != 0 || a.GetFiredMs() != 0 || a.GetAcknowledgedBy() != "" || a.GetSummary() != up.Summary {
		t.Fatalf("cleared alarm kept its episode: %v", a)
	}

	// A condition that clears during the hold never fires.
	a = a.Next(down, hold, clearAt).Next(up, hold, clearAt.Add(time.Minute))
	if a.GetState() != AlarmState_ALARM_STATE_CLEAR || a.GetFiredMs() != 0 {
		t.Fatalf("alarm cleared during its hold: %v", a)
	}

	// With no hold time the Alarm fires at once.
	a = (*Alarm)(nil).Next(down, 0, start)
	if a.GetState() != AlarmState_ALARM_STATE_FIRING {
		t.Fatalf("alarm with no hold: %v", a)
	}
}

// TestAlarmWorld writes, reads and acknowledges an Alarm in a World.
func TestAlarmWorld(t *testing.T) {
	// Start a World.
	ctx := context.Background()
	tb := world_testbed.MustDefault(t, ctx)
	const key = "alarms/forge-down"
	now := time.UnixMilli(1_793_000_000_000)

	// A missing Alarm reads as nil, and writing it creates an alarm object.
	fired := (*Alarm)(nil).Next(&Condition{Firing: true, Summary: "forge is down"}, 0, now)
	err := world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, tx world.WorldState) error {
		a, err := ReadAlarm(ctx, tx, key)
		if err != nil {
			return err
		}
		if a != nil {
			t.Fatalf("missing alarm read as %v", a)
		}
		return WriteAlarm(ctx, tx, key, fired)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Acknowledge it in a second transaction.
	err = world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, tx world.WorldState) error {
		return AcknowledgeAlarm(ctx, tx, key, "cjs", now)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read it back typed.
	err = world.ExecTransaction(ctx, tb.Engine, false, func(ctx context.Context, tx world.WorldState) error {
		// The object is an alarm.
		typeID, err := world_types.GetObjectType(ctx, tx, key)
		if err != nil {
			return err
		}
		if typeID != AlarmTypeID {
			t.Fatalf("alarm object type %q", typeID)
		}

		// It holds the acknowledged Alarm.
		a, err := ReadAlarm(ctx, tx, key)
		if err != nil {
			return err
		}
		if a.GetState() != AlarmState_ALARM_STATE_ACKNOWLEDGED || a.GetSummary() != "forge is down" {
			t.Fatalf("stored alarm: %v", a)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
