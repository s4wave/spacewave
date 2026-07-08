//go:build !js

package devtool

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

func TestDevtoolBusInstancesCoordinateWorldWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repoRoot := t.TempDir()
	stateRoot := filepath.Join(repoRoot, ".bldr")
	le := logrus.NewEntry(logrus.New())

	firstBus, err := BuildDevtoolBus(ctx, le, repoRoot, stateRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if firstBus != nil {
			firstBus.Release()
		}
	}()

	secondBus, err := BuildDevtoolBus(ctx, le, repoRoot, stateRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	defer secondBus.Release()

	prefix := "test/devtool-coord/"
	firstKey := prefix + "first-writer"
	secondKey := prefix + "second-writer"
	handoverKey := prefix + "handover-writer"

	firstRev := createDevtoolCoordObject(t, ctx, firstBus.GetWorldEngine(), firstKey)
	assertDevtoolCoordObjectRev(t, ctx, secondBus.GetWorldEngine(), firstKey, firstRev)

	assertDevtoolCoordObjectRev(t, ctx, firstBus.GetWorldEngine(), firstKey, firstRev)

	firstTx, err := firstBus.GetWorldEngine().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	firstHeldKey := prefix + "first-held-writer"
	firstHeldObj, err := firstTx.CreateObject(ctx, firstHeldKey, nil)
	if err != nil {
		firstTx.Discard()
		t.Fatal(err)
	}
	firstHeldRev, err := firstHeldObj.IncrementRev(ctx)
	if err != nil {
		firstTx.Discard()
		t.Fatal(err)
	}

	secondWriterStarted := make(chan struct{})
	secondWriterTx := make(chan world.Tx, 1)
	secondWriterErr := make(chan error, 1)
	go func() {
		close(secondWriterStarted)
		tx, err := secondBus.GetWorldEngine().NewTransaction(ctx, true)
		if err != nil {
			secondWriterErr <- err
			return
		}
		secondWriterTx <- tx
	}()
	<-secondWriterStarted

	select {
	case err := <-secondWriterErr:
		firstTx.Discard()
		t.Fatalf("second writer failed while waiting for first writer lease: %v", err)
	case tx := <-secondWriterTx:
		tx.Discard()
		firstTx.Discard()
		t.Fatal("second writer acquired while first writer held coordinator lease")
	case <-time.After(100 * time.Millisecond):
	}

	if err := firstTx.Commit(ctx); err != nil {
		firstTx.Discard()
		t.Fatal(err)
	}

	var secondTx world.Tx
	select {
	case err := <-secondWriterErr:
		t.Fatalf("second writer failed after first writer commit: %v", err)
	case secondTx = <-secondWriterTx:
	case <-time.After(5 * time.Second):
		t.Fatal("second writer did not acquire after first writer committed")
	}

	secondObj, err := secondTx.CreateObject(ctx, secondKey, nil)
	if err != nil {
		secondTx.Discard()
		t.Fatal(err)
	}
	secondRev, err := secondObj.IncrementRev(ctx)
	if err != nil {
		secondTx.Discard()
		t.Fatal(err)
	}
	if err := secondTx.Commit(ctx); err != nil {
		secondTx.Discard()
		t.Fatal(err)
	}
	assertDevtoolCoordObjectRev(t, ctx, firstBus.GetWorldEngine(), firstHeldKey, firstHeldRev)
	assertDevtoolCoordObjectRev(t, ctx, firstBus.GetWorldEngine(), secondKey, secondRev)
	assertDevtoolCoordObjectRev(t, ctx, secondBus.GetWorldEngine(), secondKey, secondRev)
	assertDevtoolCoordObjectRev(t, ctx, secondBus.GetWorldEngine(), firstHeldKey, firstHeldRev)

	firstBus.Release()
	firstBus = nil

	handoverRev := createDevtoolCoordObject(t, ctx, secondBus.GetWorldEngine(), handoverKey)
	assertDevtoolCoordObjectRev(t, ctx, secondBus.GetWorldEngine(), handoverKey, handoverRev)
}

func createDevtoolCoordObject(tb testing.TB, ctx context.Context, eng world.Engine, key string) uint64 {
	tb.Helper()

	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		tb.Fatal(err)
	}
	obj, err := tx.CreateObject(ctx, key, nil)
	if err != nil {
		tx.Discard()
		tb.Fatal(err)
	}
	rev, err := obj.IncrementRev(ctx)
	if err != nil {
		tx.Discard()
		tb.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		tx.Discard()
		tb.Fatal(err)
	}
	return rev
}

func assertDevtoolCoordObjectRev(tb testing.TB, ctx context.Context, eng world.Engine, key string, wantRev uint64) {
	tb.Helper()

	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		tb.Fatal(err)
	}
	defer tx.Discard()

	obj, found, err := tx.GetObject(ctx, key)
	if err != nil {
		tb.Fatal(err)
	}
	if !found {
		tb.Fatalf("object %q was not found", key)
	}
	_, gotRev, err := obj.GetRootRef(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	if gotRev != wantRev {
		tb.Fatalf("object %q revision = %d, want %d", key, gotRev, wantRev)
	}
}

