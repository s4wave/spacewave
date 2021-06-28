package world

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestOperationController runs the OperationController to test.
func TestOperationController(t *testing.T) {
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
	var ncalled uint32
	testCtrl := NewOperationController(
		"hydra/world/operation/test/1",
		engineID, "",
		[]ApplyWorldOpFunc{func(
			ctx context.Context,
			worldHandle WorldState,
			operationTypeID string,
			op Operation,
			opSender peer.ID,
		) (handled bool, err error) {
			atomic.AddUint32(&ncalled, 1)
			return true, nil
		}},
		[]ApplyObjectOpFunc{func(
			ctx context.Context,
			objectHandle ObjectState,
			operationTypeID string,
			op Operation,
			opSender peer.ID,
		) (handled bool, err error) {
			atomic.AddUint32(&ncalled, 1)
			return true, nil
		}},
	)

	b := tb.Bus
	go b.ExecuteController(ctx, testCtrl)

	// allow it to start
	<-time.After(time.Millisecond * 100)

	applyObjOpFn := BuildApplyObjectOpFunc(b, le, engineID)
	applyWorldOpFn := BuildApplyWorldOpFunc(b, le, engineID)

	operationTypeID := "test-operation"
	handled, err := applyObjOpFn(ctx, nil, operationTypeID, nil, "")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !handled {
		t.Fatal("expected object op to be handled")
	}
	handled, err = applyWorldOpFn(ctx, nil, operationTypeID, nil, "")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !handled {
		t.Fatal("expected world op to be handled")
	}

	nc := atomic.LoadUint32(&ncalled)
	if nc != 2 {
		t.Fatalf("expected %d calls but got %d", 2, nc)
	}

	// success
}
