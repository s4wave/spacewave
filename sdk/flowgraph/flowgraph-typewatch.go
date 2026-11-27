package s4wave_flowgraph

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
)

// typeWatches holds one object type registry watch for each type a graph's
// bound objects have. A registration or release of any watched type sends one
// wake-up, so a stream of graph snapshots can re-derive its bindings without
// polling.
type typeWatches struct {
	// b is the bus the registry answers on. b may be nil, in which case no
	// types are watched.
	b bus.Bus
	// engineID scopes each lookup to the granting World engine.
	engineID string
	// wake holds a pending registry change. It has room for one, since a second
	// change before the reader looks needs no second read.
	wake chan struct{}
	// releases holds the function that stops the watch of each type ID.
	// Only the stream's own goroutine reads or writes it.
	releases map[string]func()
}

// newTypeWatches creates an empty set of watches.
func newTypeWatches(b bus.Bus, engineID string) *typeWatches {
	return &typeWatches{
		b:        b,
		engineID: engineID,
		wake:     make(chan struct{}, 1),
		releases: make(map[string]func()),
	}
}

// notify records a registry change without blocking the registry's callback.
func (w *typeWatches) notify(*s4wave_objecttype_registry.ObjectTypeMetadata) {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// sync watches the type of each object the snapshot binds and stops watching
// types the snapshot no longer binds. It reports whether it added a watch, in
// which case the caller reads the bindings again: a registry change before the
// watch began was not delivered. Adding watches discards the wake-ups they
// produced for the registrations already present.
func (w *typeWatches) sync(ctx context.Context, snapshot *FlowgraphSnapshot) (bool, error) {
	if w.b == nil {
		return false, nil
	}
	want := make(map[string]struct{})
	for _, binding := range snapshot.GetBindings() {
		if typeID := binding.GetTypeId(); typeID != "" {
			want[typeID] = struct{}{}
		}
	}
	for typeID, release := range w.releases {
		if _, ok := want[typeID]; !ok {
			release()
			delete(w.releases, typeID)
		}
	}
	var added bool
	for typeID := range want {
		if _, ok := w.releases[typeID]; ok {
			continue
		}
		release, err := s4wave_objecttype_registry.ExWatchObjectTypeMetadata(ctx, w.b, typeID, w.engineID, w.notify)
		if err != nil {
			return false, err
		}
		w.releases[typeID] = release
		added = true
	}
	if added {
		select {
		case <-w.wake:
		default:
		}
	}
	return added, nil
}

// close stops every watch.
func (w *typeWatches) close() {
	for typeID, release := range w.releases {
		release()
		delete(w.releases, typeID)
	}
}
