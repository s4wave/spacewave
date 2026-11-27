package flowgraph_run_test

import (
	"context"
	"strconv"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/world"
	flowgraph_eventsource "github.com/s4wave/spacewave/sdk/flowgraph/eventsource"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// counterType is the object type of the counter source.
const counterType = "test/counter-source"

// counterSource is an event source whose events are the numbers 1, 2, 3 and so
// on. Its cursor is the decimal number of the last event a batch holds, and a
// test emits events by raising the count.
type counterSource struct {
	// bcast wakes the open streams when the count rises and the test when the
	// number of open streams changes.
	bcast broadcast.Broadcast
	// count is the number of the last event emitted. It is guarded by bcast.
	count int
	// watching is the number of open WatchEvents streams. It is guarded by
	// bcast.
	watching int
}

// emit adds n events.
func (s *counterSource) emit(n int) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.count += n
		broadcast()
	})
}

// waitWatching waits until exactly n WatchEvents streams are open.
func (s *counterSource) waitWatching(ctx context.Context, n int) error {
	for {
		var watching int
		var wait <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			watching, wait = s.watching, getWait()
		})
		if watching == n {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wait:
		}
	}
}

// WatchEvents sends the events after the cursor, then each batch as events are
// emitted, until the stream's context ends.
func (s *counterSource) WatchEvents(req *flowgraph_eventsource.WatchEventsRequest, strm flowgraph_eventsource.SRPCEventSourceService_WatchEventsStream) error {
	// Count the open stream until it ends.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.watching++
		broadcast()
	})
	defer s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.watching--
		broadcast()
	})

	// Resume after the cursor. An empty cursor starts from the beginning.
	sent := 0
	if len(req.GetCursor()) != 0 {
		var err error
		if sent, err = strconv.Atoi(string(req.GetCursor())); err != nil {
			return err
		}
	}

	// Send each new event, then wait for more.
	for {
		var count int
		var wait <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			count, wait = s.count, getWait()
		})
		if count > sent {
			batch := &flowgraph_eventsource.WatchEventsResponse{NextCursor: []byte(strconv.Itoa(count))}
			for n := sent + 1; n <= count; n++ {
				batch.Events = append(batch.Events, &flowgraph_eventsource.SourceEvent{EventId: "e" + strconv.Itoa(n), Port: "event"})
			}
			if err := strm.Send(batch); err != nil {
				return err
			}
			sent = count
		}
		select {
		case <-strm.Context().Done():
			return strm.Context().Err()
		case <-wait:
		}
	}
}

// _ is a type assertion
var _ flowgraph_eventsource.SRPCEventSourceServiceServer = (*counterSource)(nil)

// counterTypes registers counterType on the bus the way a plugin does: it
// answers the lookups of the type's metadata and of its resource factory.
type counterTypes struct {
	// source is the one source every object of the type serves.
	source *counterSource
}

// GetControllerInfo returns information about the controller.
func (c *counterTypes) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/counter-types", controller.MustParseVersion("0.0.1"), "counter source object type")
}

// Execute returns at once, since the controller only handles directives.
func (c *counterTypes) Execute(context.Context) error {
	return nil
}

// HandleDirective answers the lookups for counterType.
func (c *counterTypes) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case s4wave_objecttype_registry.LookupObjectTypeMetadata:
		if dir.LookupObjectTypeMetadataTypeID() != counterType {
			return nil, nil
		}
		metadata := &s4wave_objecttype_registry.ObjectTypeMetadata{DisplayName: "Counter source", Ports: []*s4wave_objecttype_registry.ObjectTypePort{
			{Name: "event", Direction: s4wave_objecttype_registry.ObjectTypePortDirection_OBJECT_TYPE_PORT_DIRECTION_OUTPUT, TypeId: "activation"},
		}}
		return directive.R(directive.NewValueResolver([]s4wave_objecttype_registry.LookupObjectTypeMetadataValue{metadata}), nil)
	case objecttype.LookupObjectType:
		if dir.LookupObjectTypeID() != counterType {
			return nil, nil
		}
		return directive.R(directive.NewValueResolver([]objecttype.LookupObjectTypeValue{objecttype.NewObjectType(counterType, c.newResource)}), nil)
	}
	return nil, nil
}

// newResource serves the source's EventSourceService for any object of the type.
func (c *counterTypes) newResource(context.Context, *logrus.Entry, bus.Bus, world.Engine, world.WorldState, string) (srpc.Invoker, func(), error) {
	mux := srpc.NewMux()
	if err := flowgraph_eventsource.SRPCRegisterEventSourceService(mux, c.source); err != nil {
		return nil, nil, err
	}
	return mux, func() {}, nil
}

// Close releases nothing.
func (c *counterTypes) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*counterTypes)(nil)
