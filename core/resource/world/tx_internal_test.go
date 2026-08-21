//go:build !js

package resource_world

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

type gateLocker struct {
	mux       sync.Mutex
	attempted chan struct{}
	acquired  chan struct{}
}

func (l *gateLocker) Lock() {
	l.attempted <- struct{}{}
	l.mux.Lock()
	l.acquired <- struct{}{}
}

func (l *gateLocker) Unlock() {
	l.mux.Unlock()
}

type terminalLockProofTx struct {
	world.Tx
	middleStarted chan struct{}
	unblockMiddle chan struct{}
	createCalls   int
}

func (tx *terminalLockProofTx) CreateObject(
	ctx context.Context,
	key string,
	rootRef *bucket.ObjectRef,
) (world.ObjectState, error) {
	tx.createCalls++
	if tx.createCalls == 2 {
		close(tx.middleStarted)
		select {
		case <-tx.unblockMiddle:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return tx.Tx.CreateObject(ctx, key, rootRef)
}

func TestTxResourceTerminalLockSerializesCommitAndDiscard(t *testing.T) {
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	baseTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	tx := &terminalLockProofTx{
		Tx:            baseTx,
		middleStarted: make(chan struct{}),
		unblockMiddle: make(chan struct{}),
	}
	resource := NewTxResource(nil, nil, tx, nil, nil)
	locker := &gateLocker{
		attempted: make(chan struct{}, 3),
		acquired:  make(chan struct{}, 3),
	}
	resource.terminalLocker = locker

	batchDone := make(chan error, 1)
	go func() {
		_, err := resource.CommitMutations(ctx, &s4wave_world.CommitMutationsRequest{
			Mutations: []*s4wave_world.TransactionMutation{
				{Mutation: &s4wave_world.TransactionMutation_CreateObject{CreateObject: &s4wave_world.CreateObjectRequest{ObjectKey: "terminal-lock/a"}}},
				{Mutation: &s4wave_world.TransactionMutation_CreateObject{CreateObject: &s4wave_world.CreateObjectRequest{ObjectKey: "terminal-lock/b"}}},
			},
		})
		batchDone <- err
	}()
	waitChannel(t, locker.attempted, "batch terminal lock attempt")
	waitChannel(t, locker.acquired, "batch terminal lock acquisition")
	waitChannel(t, tx.middleStarted, "blocked middle mutation")

	commitDone := make(chan error, 1)
	go func() {
		_, err := resource.Commit(ctx, &s4wave_world.CommitRequest{})
		commitDone <- err
	}()
	waitChannel(t, locker.attempted, "Commit terminal lock attempt")
	assertNoChannel(t, locker.acquired, "Commit terminal lock acquisition")

	discardDone := make(chan error, 1)
	go func() {
		_, err := resource.Discard(ctx, &s4wave_world.DiscardRequest{})
		discardDone <- err
	}()
	waitChannel(t, locker.attempted, "Discard terminal lock attempt")
	assertNoChannel(t, locker.acquired, "Discard terminal lock acquisition")

	close(tx.unblockMiddle)
	if err := <-batchDone; err != nil {
		t.Fatalf("CommitMutations: %v", err)
	}
	waitChannel(t, locker.acquired, "one waiting terminal operation")
	waitChannel(t, locker.acquired, "other waiting terminal operation")
	if err := <-commitDone; err == nil {
		t.Fatal("Commit succeeded after CommitMutations closed the transaction")
	}
	if err := <-discardDone; err != nil {
		t.Fatalf("Discard: %v", err)
	}
}

func waitChannel(t *testing.T, ch <-chan struct{}, action string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", action)
	}
}

func assertNoChannel(t *testing.T, ch <-chan struct{}, action string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("unexpected %s", action)
	default:
	}
}
