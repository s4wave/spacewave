//go:build !js

package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// queuedReclaimBackend announces exclusive reclamation before it waits for readers.
type queuedReclaimBackend struct {
	// Backend supplies the real disk fixture and shared lock behavior.
	Backend
	// queued reports an exclusive reclamation request before lock acquisition.
	queued chan<- struct{}
}

// Lock exposes the point at which reclamation has joined the backend lock queue.
func (b *queuedReclaimBackend) Lock(ctx context.Context, name string, exclusive bool) (func(), error) {
	if name == "reclaim" && exclusive && b.queued != nil {
		select {
		case b.queued <- struct{}{}:
		default:
		}
	}
	return b.Backend.Lock(ctx, name, exclusive)
}

// TestScopedReadSurvivesQueuedReclaimAndDrain proves nested work reuses its lease.
func TestScopedReadSurvivesQueuedReclaimAndDrain(t *testing.T) {
	// Open an engine whose backend exposes queued reclamation.
	ctx := t.Context()
	disk := newDiskBackend(t)
	backend := &queuedReclaimBackend{Backend: disk}
	engine, err := Open(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	// Publish a durable block before opening the scoped reader.
	durable := &packStore{engine: engine}
	durableRef, existed, err := durable.PutBlock(ctx, []byte("durable content"), &block.PutOpts{Sync: true})
	if err != nil || existed {
		t.Fatalf("durable seed: %t %v", existed, err)
	}

	// Hold another block at the publication gate and await its write attempt.
	writeGate := make(chan struct{})
	disk.writeGate = writeGate
	disk.writeStarted = make(chan struct{}, 1)
	store := NewBlockStore(ctx, engine, 0)
	defer store.Close()
	pendingRef, existed, err := store.PutBlock(ctx, []byte("pending content"), nil)
	if err != nil || existed {
		t.Fatalf("pending admission: %t %v", existed, err)
	}
	select {
	case <-disk.writeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("pending publication did not reach the write gate")
	}

	// Retain an outer read scope over durable and pending blocks.
	outer, releaseOuter, err := store.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { releaseOuter() }()

	// Queue reclamation behind the retained read scope.
	reclaimQueued := make(chan struct{}, 1)
	backend.queued = reclaimQueued
	reclaimed := make(chan error, 1)
	go func() {
		_, err := engine.Reclaim(ctx)
		reclaimed <- err
	}()
	select {
	case <-reclaimQueued:
	case <-time.After(5 * time.Second):
		t.Fatal("reclamation did not join the lock queue")
	}

	// Verify a nested read reuses the outer scope while reclamation waits.
	nestedDone := make(chan error, 1)
	go func() {
		// Acquire the nested scope and verify durable and pending block membership.
		nested, releaseNested, err := outer.BeginReadOperation(ctx)
		if err != nil {
			nestedDone <- err
			return
		}
		defer releaseNested()
		exists, err := nested.GetBlockExistsBatch(ctx, []*block.BlockRef{durableRef, pendingRef})
		if err == nil && (len(exists) != 2 || !exists[0] || !exists[1]) {
			err = errors.New("nested batch did not find durable and pending blocks")
		}
		nestedDone <- err
	}()
	select {
	case err := <-nestedDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nested scoped read queued behind reclamation")
	}

	// Resume publication and require a scoped durability fence to finish.
	synced := make(chan error, 1)
	go func() {
		ok, err := outer.Sync(ctx)
		if err == nil && !ok {
			err = errors.New("scoped sync did not report a durability fence")
		}
		synced <- err
	}()
	close(writeGate)
	select {
	case err := <-synced:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scoped sync did not complete after publication resumed")
	}

	// Verify pending bytes survive drain and reclamation resumes after release.
	data, found, err := outer.GetBlock(ctx, pendingRef)
	if err != nil || !found || string(data) != "pending content" {
		t.Fatalf("captured pending read after drain: %q %t %v", data, found, err)
	}
	releaseOuter()
	releaseOuter = func() {}
	select {
	case err := <-reclaimed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reclamation did not complete after scoped read released")
	}
}
