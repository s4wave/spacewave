package world

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestLookupOpController runs the LookupOpController to test.
func TestLookupOpController(t *testing.T) {
	// Configure debug logging for the World operation controller test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Open the testbed that hosts the World operation controller.
	testbed.Verbose = false
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Construct a controller that counts resolved World operation requests.
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

	// Execute the World operation controller on the testbed bus.
	b := tb.Bus
	go func() {
		_ = b.ExecuteController(ctx, testCtrl)
	}()

	// allow it to start
	<-time.After(time.Millisecond * 100)

	// Bind operation lookups to the controller engine target.
	lookupWorldOpFn := BuildLookupWorldOpFunc(b, le, engineID)

	// Resolve one World operation and verify its callback count.
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
