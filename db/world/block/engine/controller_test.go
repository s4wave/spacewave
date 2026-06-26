package world_block_engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	bdberrors "github.com/aperturerobotics/bbolt/errors"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/kvtx"
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

func TestRefreshHeadFromCoordinatorEventIgnoresClosedStore(t *testing.T) {
	log := logrus.New()
	hook := &entriesHook{}
	log.AddHook(hook)
	ctrl := &Controller{le: logrus.NewEntry(log)}

	ctrl.refreshHeadFromCoordinatorEvent(context.Background(), closedHeadStore{}, nil, coord.Event{
		Generation: 1,
	})

	for _, entry := range hook.entries {
		if entry.Message == "world head refresh failed" ||
			strings.Contains(entry.Message, bdberrors.ErrDatabaseNotOpen.Error()) {
			t.Fatalf("closed head store produced warning: level=%s message=%q data=%v", entry.Level, entry.Message, entry.Data)
		}
		if err, ok := entry.Data[logrus.ErrorKey].(error); ok && errors.Is(err, bdberrors.ErrDatabaseNotOpen) {
			t.Fatalf("closed head store error was logged: level=%s message=%q data=%v", entry.Level, entry.Message, entry.Data)
		}
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

type closedHeadStore struct{}

func (closedHeadStore) NewTransaction(context.Context, bool) (kvtx.Tx, error) {
	return nil, bdberrors.ErrDatabaseNotOpen
}

type entriesHook struct {
	entries []*logrus.Entry
}

func (h *entriesHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *entriesHook) Fire(entry *logrus.Entry) error {
	h.entries = append(h.entries, entry)
	return nil
}
