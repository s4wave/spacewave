//go:build !js

package devtool

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestDevtoolBusReleaseOrderAndExactlyOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var order []string
	recordRelease := func(name string) func() {
		return func() {
			select {
			case <-ctx.Done():
				order = append(order, name)
			default:
				t.Fatalf("%s released before context cancellation", name)
			}
		}
	}
	rels := []func(){recordRelease("first"), recordRelease("second")}
	release := newDevtoolBusRelease(cancel, &rels, func() {
		select {
		case <-ctx.Done():
			order = append(order, "state-lock")
		default:
			t.Fatal("state lock released before context cancellation")
		}
	})

	release()
	release()
	want := []string{"second", "first", "state-lock"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("release order = %v, want %v", order, want)
	}
}

func TestDevtoolBusReleaseCancelsContextAndUnlocksState(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	le := logrus.NewEntry(logrus.New())
	bus, err := BuildDevtoolBus(context.Background(), le, repoRoot, stateRoot, false)
	if err != nil {
		t.Fatalf("build devtool bus: %v", err)
	}

	bus.Release()
	bus.Release()
	select {
	case <-bus.GetContext().Done():
	case <-time.After(time.Second):
		t.Fatal("released bus context remained active")
	}

	reacquireCtx, reacquireCancel := context.WithTimeout(context.Background(), time.Second)
	defer reacquireCancel()
	nextBus, err := BuildDevtoolBus(reacquireCtx, le, repoRoot, stateRoot, false)
	if err != nil {
		t.Fatalf("build devtool bus after release: %v", err)
	}
	nextBus.Release()
}
