package s4wave_world

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

func TestRemoteObjectStateAccessWorldStateReportsUnavailable(t *testing.T) {
	obj := &ObjectState{}
	called := false
	err := obj.AccessWorldState(context.Background(), nil, func(*world.WorldAccess) error {
		called = true
		return nil
	})
	if !errors.Is(err, world.ErrWorldStorageUnavailable) {
		t.Fatalf("AccessWorldState error = %v, want %v", err, world.ErrWorldStorageUnavailable)
	}
	if called {
		t.Fatal("AccessWorldState called callback without a remote cursor adapter")
	}
}
