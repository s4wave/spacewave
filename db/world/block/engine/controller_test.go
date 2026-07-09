package world_block_engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bdberrors "github.com/aperturerobotics/bbolt/errors"
	"github.com/s4wave/spacewave/db/block"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
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

func TestControllerGetWorldEngineReturnsMissingInitHeadError(t *testing.T) {
	ctx := t.Context()
	log := logrus.New()
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	missingRootRef := controllerTestBlockRef(t, "world-engine-missing-init-head")
	conf := NewConfig(
		"test-world-engine-missing-init-head",
		tb.Volume.GetID(),
		tb.BucketId,
		"",
		&bucket.ObjectRef{
			BucketId: tb.BucketId,
			RootRef:  missingRootRef,
		},
		nil,
		false,
	)
	ctrl, err := NewController(le, tb.Bus, conf, transform_all.BuildFactorySet())
	if err != nil {
		t.Fatal(err.Error())
	}

	execCtx, execCancel := context.WithCancel(ctx)
	defer execCancel()
	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- ctrl.Execute(execCtx)
	}()

	getCtx, getCancel := context.WithTimeout(ctx, 2*time.Second)
	defer getCancel()
	eng, err := ctrl.GetWorldEngine(getCtx)
	if eng != nil {
		t.Fatalf("GetWorldEngine returned engine %T, want nil with fatal startup error", eng)
	}
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("GetWorldEngine error = %v, want block.ErrNotFound", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetWorldEngine waited until the guard context expired: %v", err)
	}

	select {
	case err := <-execErrCh:
		if err != nil {
			t.Fatalf("Execute error = %v, want nil after publishing fatal startup error", err)
		}
	case <-getCtx.Done():
		t.Fatalf("Execute did not return after publishing fatal startup error: %v", getCtx.Err())
	}
}

func controllerTestBlockRef(t *testing.T, data string) *block.BlockRef {
	t.Helper()
	h, err := hash.Sum(hash.HashType_HashType_BLAKE3, []byte(data))
	if err != nil {
		t.Fatal(err.Error())
	}
	return block.NewBlockRef(h)
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
