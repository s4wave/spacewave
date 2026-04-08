package world

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestLookupOpController runs the LookupOpController to test.
func TestLookupOpController(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testbed.Verbose = false
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	engineID := "test-engine"
	var ncalled atomic.Uint32
	testCtrl := NewLookupOpController(
		"hydra/world/operation/test",
		engineID,
		func(ctx context.Context, opTypeID string) (Operation, error) {
			ncalled.Add(1)
			return nil, nil
		},
	)

	b := tb.Bus
	go func() {
		_ = b.ExecuteController(ctx, testCtrl)
	}()

	// allow it to start
	<-time.After(time.Millisecond * 100)

	lookupWorldOpFn := BuildLookupWorldOpFunc(b, le, engineID)

	operationTypeID := "test-operation"
	op, err := lookupWorldOpFn(ctx, operationTypeID)
	if err != nil {
		t.Fatal(err.Error())
	}
	if op != nil {
		t.Fatal("expected object op to be nil")
	}
	nc := ncalled.Load()
	if nc != 1 {
		t.Fatalf("expected %d calls but got %d", 1, nc)
	}

	// success
}
