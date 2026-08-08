package cluster_controller

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

func TestSkipUnhandledOperation(t *testing.T) {
	handler := skipUnhandledOperation(func(
		context.Context,
		*logrus.Entry,
		world.WorldState,
		world.ObjectState,
		*bucket.ObjectRef,
		uint64,
	) (bool, error) {
		return false, world.ErrUnhandledOp
	})

	waitForChanges, err := handler(context.Background(), nil, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if !waitForChanges {
		t.Fatal("waitForChanges = false, want true")
	}
}
