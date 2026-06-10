package world_block_engine

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/sirupsen/logrus"
)

func TestControllerCoordinatorSupported(t *testing.T) {
	ctx := context.Background()
	ctrl := &Controller{le: logrus.NewEntry(logrus.New())}
	scope := coord.Scope{
		VolumeID:      "volume",
		ObjectStoreID: "store",
		ParticipantID: "engine",
	}

	if !ctrl.coordinatorSupported(ctx, fakeCoordinator{capability: &coord.Capability{Supported: true}}, scope) {
		t.Fatal("supported coordinator reported false")
	}
	if ctrl.coordinatorSupported(ctx, fakeCoordinator{capability: &coord.Capability{Supported: false}}, scope) {
		t.Fatal("unsupported coordinator reported true")
	}
	if ctrl.coordinatorSupported(ctx, fakeCoordinator{err: coord.ErrUnsupported}, scope) {
		t.Fatal("errored coordinator reported true")
	}
}

type fakeCoordinator struct {
	capability *coord.Capability
	err        error
}

func (f fakeCoordinator) Capability(context.Context, coord.Scope) (*coord.Capability, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.capability, nil
}

func (fakeCoordinator) Snapshot(context.Context, coord.Scope) (*coord.Snapshot, error) {
	return nil, coord.ErrUnsupported
}

func (fakeCoordinator) Watch(context.Context, coord.Scope, uint64) (coord.Watch, error) {
	return nil, coord.ErrUnsupported
}

func (fakeCoordinator) TryAcquireWriteLease(context.Context, coord.Scope) (coord.WriteLease, bool, error) {
	return nil, false, coord.ErrUnsupported
}

func (fakeCoordinator) WaitAcquireWriteLease(context.Context, coord.Scope) (coord.WriteLease, error) {
	return nil, coord.ErrUnsupported
}

var _ coord.Coordinator = fakeCoordinator{}
