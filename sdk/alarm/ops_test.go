package s4wave_alarm

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
)

// TestAcknowledgeAlarmOp acknowledges a firing Alarm as the op's sender and
// rejects a second acknowledgement.
func TestAcknowledgeAlarmOp(t *testing.T) {
	// Start a World holding a firing Alarm.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	key, now := "alarms/forge-down", time.UnixMilli(1_793_000_000_000)
	fired := (*Alarm)(nil).Next(&Condition{Firing: true, Summary: "forge is down"}, 0, now)
	execOp(t, ctx, tb, func(ctx context.Context, tx world.WorldState) error {
		return WriteAlarm(ctx, tx, key, fired)
	})

	// Ops without an Alarm or a time are rejected.
	for _, op := range []*AcknowledgeAlarmOp{{TimeMs: now.UnixMilli()}, {ObjectKey: key}} {
		if op.Validate() == nil {
			t.Fatalf("invalid op accepted: %v", op)
		}
	}

	// The op acknowledges the Alarm as its sender at its time.
	op := &AcknowledgeAlarmOp{ObjectKey: key, TimeMs: now.Add(time.Minute).UnixMilli()}
	sender := peer.ID("12D3KooWSender")
	execOp(t, ctx, tb, func(ctx context.Context, tx world.WorldState) error {
		_, err := op.ApplyWorldOp(ctx, nil, tx, sender)
		return err
	})
	a, err := ReadAlarm(ctx, tb.WorldState, key)
	if err != nil {
		t.Fatal(err)
	}
	if a.GetState() != AlarmState_ALARM_STATE_ACKNOWLEDGED || a.GetAcknowledgedBy() != sender.String() || a.GetAcknowledgedMs() != op.GetTimeMs() {
		t.Fatalf("acknowledged alarm: %v", a)
	}

	// A second acknowledgement finds the Alarm no longer firing.
	err = world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, tx world.WorldState) error {
		_, err := op.ApplyWorldOp(ctx, nil, tx, sender)
		return err
	})
	if !errors.Is(err, ErrNotFiring) {
		t.Fatalf("second acknowledgement: %v", err)
	}
}

// TestSetReads points an Alarm's reads edges at one set of objects, then
// another.
func TestSetReads(t *testing.T) {
	// Start a World holding an Alarm and three objects.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	key := "alarms/forge-down"
	execOp(t, ctx, tb, func(ctx context.Context, tx world.WorldState) error {
		if err := WriteAlarm(ctx, tx, key, &Alarm{}); err != nil {
			return err
		}
		for _, obj := range []string{"a", "b", "c"} {
			o, _, err := world.CreateWorldObject(ctx, tx, obj, func(*block.Cursor) error { return nil })
			world.ReleaseObjectState(o)
			if err != nil {
				return err
			}
		}
		return nil
	})

	// Each call leaves exactly the edges it names.
	for _, want := range [][]string{{"a", "b"}, {"b", "c"}, nil} {
		execOp(t, ctx, tb, func(ctx context.Context, tx world.WorldState) error {
			return SetReads(ctx, tx, key, want)
		})
		if got := readsOf(t, ctx, tb, key); !slices.Equal(got, want) {
			t.Fatalf("reads edges %v, want %v", got, want)
		}
	}
}

// execOp runs fn in a write transaction.
func execOp(t *testing.T, ctx context.Context, tb *world_testbed.Testbed, fn func(ctx context.Context, tx world.WorldState) error) {
	t.Helper()
	if err := world.ExecTransaction(ctx, tb.Engine, true, fn); err != nil {
		t.Fatal(err)
	}
}

// readsOf returns the sorted keys the Alarm key's reads edges point at.
func readsOf(t *testing.T, ctx context.Context, tb *world_testbed.Testbed, key string) []string {
	// Look up the edges.
	t.Helper()
	found, err := tb.WorldState.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, ReadsPredicate, "", ""), 0)
	if err != nil {
		t.Fatal(err)
	}

	// Collect their objects.
	var keys []string
	for _, q := range found {
		obj, err := world.GraphValueToKey(q.GetObj())
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, obj)
	}
	slices.Sort(keys)
	return keys
}
