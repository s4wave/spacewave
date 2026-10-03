package block_gc

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// TestManagerRetriesStartupReplay preserves pending ownership until replay succeeds.
func TestManagerRetriesStartupReplay(t *testing.T) {
	// Prepare the cancellable manager test lifecycle.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Create retained and orphaned objects for startup replay.
	graph := newMockGraph()
	graph.addRoot("root")
	graph.addNode(ObjectIRI("retained"))
	graph.addNode(ObjectIRI("orphan"))
	target := &mockSweepTarget{}
	replays := 0
	maintained := false

	// Configure replay failures and maintenance cancellation.
	manager := NewManager(ManagerConfig{
		Graph:      graph,
		Target:     target,
		AcquireSTW: noopSTW,
		ReplayWAL: func(ctx context.Context, graph CollectorGraph) (int, error) {
			// Count replay attempts and require pending ownership to protect objects.
			replays++
			if replays <= 3 && len(target.deletedObjects) != 0 {
				t.Fatal("swept objects before pending ownership was replayed")
			}

			// Inject the first two replay failures.
			if replays <= 2 {
				return 0, errors.New("transaction attempts exhausted")
			}

			// Restore retained-object ownership on the successful replay.
			if replays == 3 {
				return 1, graph.AddRef(ctx, "root", ObjectIRI("retained"))
			}
			return 0, nil
		},
		SweepInterval: time.Millisecond,
		Maintenance: func(context.Context) error {
			maintained = true
			cancel()
			return nil
		},
	})

	// Run the manager until its lifecycle is cancelled.
	if err := manager.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("manager returned %v, want cancellation after recovery", err)
	}

	// Verify startup recovery reaches maintenance after four replays.
	if replays != 4 || !maintained {
		t.Fatalf("replays = %d, maintained = %t, want four replays and maintenance", replays, maintained)
	}

	// Verify only the orphaned object is swept.
	if !slices.Equal(target.deletedObjects, []string{ObjectIRI("orphan")}) {
		t.Fatalf("deleted objects = %v, want only the orphan", target.deletedObjects)
	}
}

// TestManagerStartupReplayCancellation does not defer shutdown to the next cycle.
func TestManagerStartupReplayCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	manager := NewManager(ManagerConfig{
		ReplayWAL: func(context.Context, CollectorGraph) (int, error) {
			cancel()
			return 0, errors.New("replay interrupted")
		},
	})
	if err := manager.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("manager returned %v, want cancellation", err)
	}
}
