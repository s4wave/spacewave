package flowgraph_run

import (
	"context"
	"maps"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_eventsource "github.com/s4wave/spacewave/sdk/flowgraph/eventsource"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// eventSourceKey identifies one event-source node and the object it binds. A
// node bound to another object is another watcher.
type eventSourceKey struct {
	// nodeID is the event-source node of the pinned graph.
	nodeID string
	// objectKey is the World object the node binds.
	objectKey string
}

// eventSourceKeys lists the event-source nodes the standing run's pinned graph
// binds to an object. A run that is not standing has none, since only the
// standing run takes events. A missing run has none.
func eventSourceKeys(ctx context.Context, ws world.WorldState, runKey string) ([]eventSourceKey, error) {
	// Read the run and its pinned graph.
	obj, run, graph, err := readRun(ctx, ws, runKey)
	defer world.ReleaseObjectState(obj)
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !run.GetStanding() {
		return nil, nil
	}

	// Select each node that names its source object.
	var keys []eventSourceKey
	for _, id := range slices.Sorted(maps.Keys(graph.GetNodes())) {
		node := graph.GetNodes()[id]
		if node.GetTypeId() == s4wave_flowgraph.EventSourceNodeTypeID && node.GetObjectKey() != "" {
			keys = append(keys, eventSourceKey{nodeID: id, objectKey: node.GetObjectKey()})
		}
	}
	return keys, nil
}

// eventSourceWatcher reads the events of one event-source node and delivers them
// to the run. It holds a WatchEvents stream open while the node's binding reads
// BOUND, and closes it when the binding reads UNAVAILABLE or UNREGISTERED.
type eventSourceWatcher struct {
	// c is the run controller.
	c *Controller
	// key is the node and its object.
	key eventSourceKey
}

// newEventSourceWatcher constructs the watcher routine for an event-source node.
func (c *Controller) newEventSourceWatcher(key eventSourceKey) (keyed.Routine, *eventSourceWatcher) {
	w := &eventSourceWatcher{c: c, key: key}
	return w.execute, w
}

// execute follows the node's binding until the context ends or a stream fails.
// A failure ends the routine, and the controller's retry reopens it from the
// stored cursor after a backoff.
func (w *eventSourceWatcher) execute(ctx context.Context) error {
	// Follow the bound object, which reads BOUND only while its type is served.
	watch := s4wave_flowgraph.NewBindingWatch(w.c.bus, w.c.engine, w.c.conf.GetEngineId(), w.key.objectKey)
	defer watch.Close()

	// Stream while bound, and wait for the binding to change otherwise.
	binding, err := watch.Next(ctx)
	for err == nil {
		if binding.GetState() == s4wave_flowgraph.FlowgraphBindingState_FLOWGRAPH_BINDING_STATE_BOUND {
			binding, err = w.serve(ctx, watch, binding)
			continue
		}
		w.c.le.Debugf("event source %q is %s, waiting", w.key.nodeID, binding.GetState())
		binding, err = watch.Next(ctx)
	}
	return err
}

// serve streams the object's events until its binding changes, and returns the
// new binding. It returns an error when the stream fails while the binding
// stands.
func (w *eventSourceWatcher) serve(
	ctx context.Context,
	watch *s4wave_flowgraph.BindingWatch,
	binding *s4wave_flowgraph.FlowgraphBinding,
) (*s4wave_flowgraph.FlowgraphBinding, error) {
	// Cancel the stream when the binding changes.
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type change struct {
		binding *s4wave_flowgraph.FlowgraphBinding
		err     error
	}
	changed := make(chan change, 1)
	go func() {
		binding, err := watch.Next(streamCtx)
		changed <- change{binding: binding, err: err}
		cancel()
	}()

	// Stream until the stream fails or the binding change cancels it.
	streamErr := w.stream(streamCtx, binding)
	cancel()
	next := <-changed
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if next.err == nil {
		return next.binding, nil
	}
	if !errors.Is(next.err, context.Canceled) {
		return nil, next.err
	}
	return nil, streamErr
}

// stream opens WatchEvents on the object's resource from the run's stored
// cursor and delivers each batch. It opens the stream again from the stored
// cursor when a delivery names a stale cursor, and returns when the stream
// fails or the context ends.
func (w *eventSourceWatcher) stream(ctx context.Context, binding *s4wave_flowgraph.FlowgraphBinding) error {
	// Build the object's resource client.
	client, closeClient, err := w.openClient(ctx, binding.GetTypeId())
	if err != nil {
		return err
	}
	defer closeClient()

	// Resume from the stored cursor each time a delivery loses the race.
	for {
		cursor, err := w.readCursor(ctx)
		if err != nil {
			return err
		}
		if err := w.watchEvents(ctx, client, cursor, binding); err != nil {
			return err
		}
	}
}

// openClient builds the object's resource from its registered object type and
// returns an EventSourceService client for it. Call the returned function to
// release the resource and the type lookup.
func (w *eventSourceWatcher) openClient(ctx context.Context, typeID string) (flowgraph_eventsource.SRPCEventSourceServiceClient, func(), error) {
	// Resolve the object type under the run's World engine scope.
	ctx = objecttype.WithEngineID(ctx, w.c.conf.GetEngineId())
	objType, ref, err := objecttype.ExLookupObjectType(ctx, w.c.bus, typeID)
	if err != nil {
		return nil, nil, err
	}
	if objType == nil {
		return nil, nil, errors.Errorf("object type %q is not registered", typeID)
	}

	// Construct the object's resource with the type's factory.
	invoker, cleanup, err := objType.GetFactory()(ctx, w.c.le, w.c.bus, w.c.engine, world.NewEngineWorldState(w.c.engine, false), w.key.objectKey)
	if err != nil {
		ref.Release()
		return nil, nil, errors.Wrap(err, "object type factory")
	}
	release := func() {
		if cleanup != nil {
			cleanup()
		}
		ref.Release()
	}
	if invoker == nil {
		release()
		return nil, nil, errors.Errorf("object type %q serves no resource", typeID)
	}

	// Call the resource in process.
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))
	return flowgraph_eventsource.NewSRPCEventSourceServiceClient(client), release, nil
}

// watchEvents delivers the batches of one WatchEvents stream opened after
// cursor, and acknowledges each delivered one. It returns nil when a delivery
// names a stale cursor or the run holds it, so the caller resumes from the
// stored cursor once the run takes events, and an error when the stream ends
// or fails.
func (w *eventSourceWatcher) watchEvents(
	ctx context.Context,
	client flowgraph_eventsource.SRPCEventSourceServiceClient,
	cursor []byte,
	binding *s4wave_flowgraph.FlowgraphBinding,
) error {
	// Open the stream after the cursor.
	stream, err := client.WatchEvents(ctx, &flowgraph_eventsource.WatchEventsRequest{Cursor: cursor})
	if err != nil {
		return err
	}
	defer stream.Close()

	// Deliver each batch against the cursor the previous batch left.
	for {
		batch, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.Wrap(err, "event source stream")
		}
		result, err := w.deliver(ctx, cursor, batch, binding)
		if err != nil {
			return err
		}
		switch result {
		case DeliverEventsResult_DELIVER_EVENTS_RESULT_STALE_CURSOR:
			w.c.le.Debugf("event source %q: another reader moved the cursor, resuming from the stored one", w.key.nodeID)
			return nil
		case DeliverEventsResult_DELIVER_EVENTS_RESULT_HELD:
			w.c.le.Debugf("event source %q: the run holds its events, waiting", w.key.nodeID)
			return nil
		}
		cursor = batch.GetNextCursor()

		// Tell the source what the run now holds. The Space has decided the
		// delivery, and a replay cannot undo it, since every run writer
		// expects the run root it read. A failed ack only delays the source's
		// cleanup, and the next batch's ack covers it.
		if len(cursor) != 0 {
			if _, err := client.AckEvents(ctx, &flowgraph_eventsource.AckEventsRequest{Cursor: cursor}); err != nil {
				w.c.le.WithError(err).Debugf("event source %q: ack failed", w.key.nodeID)
			}
		}
	}
}

// readCursor waits until the run takes events and returns its stored cursor
// for the node. While the run holds its sources it waits on the run's next
// revision, so no stream is open and no event is read.
func (w *eventSourceWatcher) readCursor(ctx context.Context) ([]byte, error) {
	for {
		// Read the run and stop when it takes events.
		run, rev, err := w.readRun(ctx)
		if err != nil || takesEvents(run) {
			return run.GetCursor()[w.key.nodeID], err
		}

		// Wait for the run to change.
		w.c.le.Debugf("event source %q: the run is %s with its arrivals waiting, holding the source", w.key.nodeID, run.GetState())
		if _, err := world.WaitObjectRev(ctx, world.NewEngineWorldState(w.c.engine, false), w.c.conf.GetObjectKey(), rev+1, false); err != nil {
			return nil, err
		}
	}
}

// readRun reads the run and its revision in one read transaction.
func (w *eventSourceWatcher) readRun(ctx context.Context) (*s4wave_flowgraph.FlowgraphRun, uint64, error) {
	// Open the read transaction.
	tx, err := w.c.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Discard()

	// Read the run and its revision.
	obj, run, _, err := readRun(ctx, tx, w.c.conf.GetObjectKey())
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, 0, err
	}
	_, rev, err := obj.GetRootRef(ctx)
	return run, rev, err
}

// deliver hands one batch to the run in one World transaction. A batch whose
// events have no payload object names the source object.
func (w *eventSourceWatcher) deliver(
	ctx context.Context,
	cursor []byte,
	batch *flowgraph_eventsource.WatchEventsResponse,
	binding *s4wave_flowgraph.FlowgraphBinding,
) (DeliverEventsResult, error) {
	// Translate the source's events to the run's delivery.
	req := &DeliverEventsRequest{
		NodeId:     w.key.nodeID,
		Cursor:     cursor,
		NextCursor: batch.GetNextCursor(),
	}
	for _, event := range batch.GetEvents() {
		objectKey := event.GetPayloadObjectKey()
		if objectKey == "" {
			objectKey = w.key.objectKey
		}
		req.Events = append(req.Events, &DeliveredEvent{
			ObjectKey: objectKey,
			EventId:   event.GetEventId(),
			Key:       event.GetKey(),
			Port:      event.GetPort(),
			Payload:   event.GetPayload(),
		})
	}

	// Apply the delivery; a concurrent writer replays the attempt.
	var result DeliverEventsResult
	err := world.ExecTransaction(ctx, w.c.engine, true, func(ctx context.Context, tx world.WorldState) error {
		var err error
		result, err = DeliverEvents(ctx, tx, w.c.conf.GetObjectKey(), req, binding)
		return err
	})
	return result, err
}
