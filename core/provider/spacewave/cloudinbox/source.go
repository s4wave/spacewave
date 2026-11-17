package cloudinbox

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_eventsource "github.com/s4wave/spacewave/sdk/flowgraph/eventsource"
)

// maxBatchPayload bounds the payload bytes of one batch, so one delivery
// writes a bounded amount into the run however large an inbox page is.
const maxBatchPayload = 256 << 10

// Transport is the source's view of one inbox, signed as the Space's account.
type Transport interface {
	// List returns the events with a seqno above since, oldest first, with the
	// inbox's gap marker and newest seqno.
	List(ctx context.Context, since uint64) (*ListInboxEventsResponse, error)
	// Ack tells the Cloud the run stored every event through seqno.
	Ack(ctx context.Context, seqno uint64) error
	// Watch opens the inbox's event channel and calls onHead with the newest
	// seqno each time the Cloud pushes it, first when the channel opens. It
	// blocks until the channel closes or ctx ends, and returns a non-nil error.
	Watch(ctx context.Context, onHead func(seqno uint64)) error
}

// OpenTransport returns the Transport of the source's inbox. It waits while
// the account's session is locked, fails when the Space has no Spacewave
// Cloud account to sign as, and the returned function releases the Transport.
type OpenTransport func(ctx context.Context) (Transport, func(), error)

// Source serves EventSourceService for one cloud inbox. Its cursor is the
// decimal seqno of the last event the run stored. It keeps no state between
// calls: the run's stored cursor is the only position, so any Device's run
// controller may read the same inbox.
type Source struct {
	// open returns the inbox Transport.
	open OpenTransport
}

// NewSource returns the Source of an inbox reached through open.
func NewSource(open OpenTransport) *Source {
	return &Source{open: open}
}

// WatchEvents implements EventSourceService. It sends the events after the
// cursor in bounded batches, then waits on the inbox's channel and sends each
// new event. A gap the cursor has not passed comes first, as one event on the
// gap port, in the batch that moves the cursor past it.
func (s *Source) WatchEvents(req *flowgraph_eventsource.WatchEventsRequest, strm flowgraph_eventsource.SRPCEventSourceService_WatchEventsStream) error {
	// Open the inbox as the account; this waits while the session is locked.
	ctx, cancel := context.WithCancel(strm.Context())
	defer cancel()
	cursor, err := ParseCursor(req.GetCursor())
	if err != nil {
		return err
	}
	tpt, release, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Watch the channel; a head past the cursor, or any first frame after a
	// connect, wakes the reader. A failed channel ends the stream.
	wake := make(chan struct{}, 1)
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- tpt.Watch(ctx, func(uint64) {
			select {
			case wake <- struct{}{}:
			default:
			}
		})
	}()

	// Read to the inbox's end, then wait for the next head.
	for {
		if cursor, err = s.catchUp(ctx, tpt, cursor, strm); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-watchErr:
			return errors.Wrap(err, "inbox channel")
		case <-wake:
		}
	}
}

// catchUp sends every page after cursor and returns the cursor of the last
// batch sent.
func (s *Source) catchUp(ctx context.Context, tpt Transport, cursor uint64, strm flowgraph_eventsource.SRPCEventSourceService_WatchEventsStream) (uint64, error) {
	for {
		// Read the page after the cursor.
		page, err := tpt.List(ctx, cursor)
		if err != nil {
			return cursor, errors.Wrap(err, "list inbox events")
		}

		// Send it in bounded batches, the gap first.
		before := cursor
		for _, batch := range Batches(page, cursor) {
			if err := strm.Send(batch); err != nil {
				return cursor, err
			}
			if cursor, err = ParseCursor(batch.GetNextCursor()); err != nil {
				return cursor, err
			}
		}

		// Stop at the inbox's end, or at a page that made no progress.
		if cursor == before || cursor >= page.GetLastSeqno() {
			return cursor, nil
		}
	}
}

// AckEvents implements EventSourceService: it advances the inbox's ack
// cursor so the Cloud drops what the run stored. The Cloud keeps the furthest
// cursor, so acks from several Devices may arrive in any order.
func (s *Source) AckEvents(ctx context.Context, req *flowgraph_eventsource.AckEventsRequest) (*flowgraph_eventsource.AckEventsResponse, error) {
	// Parse the cursor and send it as the account.
	seqno, err := ParseCursor(req.GetCursor())
	if err != nil || seqno == 0 {
		return &flowgraph_eventsource.AckEventsResponse{}, err
	}
	tpt, release, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := tpt.Ack(ctx, seqno); err != nil {
		return nil, errors.Wrap(err, "ack inbox events")
	}
	return &flowgraph_eventsource.AckEventsResponse{}, nil
}

// Batches splits a page read after cursor into batches whose payloads total
// at most maxBatchPayload, each naming the seqno of its last event as the
// next cursor. A gap the cursor has not passed leads the first batch, and the
// events at or below the cursor are skipped.
func Batches(page *ListInboxEventsResponse, cursor uint64) []*flowgraph_eventsource.WatchEventsResponse {
	// Start the first batch with the gap, if the cursor is behind it.
	var batches []*flowgraph_eventsource.WatchEventsResponse
	batch := &flowgraph_eventsource.WatchEventsResponse{}
	var size int
	if gap := page.GetGapThrough(); gap > cursor {
		ev := gapEvent(gap, cursor)
		batch.Events = append(batch.Events, ev)
		size = len(ev.GetPayload())
		cursor = gap
		batch.NextCursor = FormatCursor(cursor)
	}

	// Append each later event, closing a batch when the next would overflow it.
	for _, inboxEvent := range page.GetEvents() {
		if inboxEvent.GetSeqno() <= cursor {
			continue
		}
		ev := sourceEvent(inboxEvent)
		if len(batch.GetEvents()) != 0 && size+len(ev.GetPayload()) > maxBatchPayload {
			batches = append(batches, batch)
			batch, size = &flowgraph_eventsource.WatchEventsResponse{}, 0
		}
		batch.Events = append(batch.Events, ev)
		size += len(ev.GetPayload())
		cursor = inboxEvent.GetSeqno()
		batch.NextCursor = FormatCursor(cursor)
	}
	if len(batch.GetEvents()) != 0 {
		batches = append(batches, batch)
	}
	return batches
}

// sourceEvent converts an inbox event into a source event on its kind's port,
// keyed by its subject. An event whose encoding exceeds the event payload
// bound drops its raw delivery body, as the inbox does for an oversized one.
func sourceEvent(ev *InboxEvent) *flowgraph_eventsource.SourceEvent {
	// Encode the event, without its body when the body makes it too large.
	payload, _ := ev.MarshalVT()
	if len(payload) > s4wave_flowgraph.MaxEventPayload {
		trimmed := ev.CloneVT()
		trimmed.Payload = nil
		payload, _ = trimmed.MarshalVT()
	}
	return &flowgraph_eventsource.SourceEvent{
		EventId: strconv.FormatUint(ev.GetSeqno(), 10),
		Key:     ev.GetSubject(),
		Port:    KindPort(ev.GetKind()),
		Payload: payload,
	}
}

// gapEvent is the one event that reports the events shed through gapThrough.
func gapEvent(gapThrough, cursor uint64) *flowgraph_eventsource.SourceEvent {
	payload, _ := (&InboxGap{GapThrough: gapThrough, Cursor: cursor}).MarshalVT()
	return &flowgraph_eventsource.SourceEvent{
		EventId: "gap/" + strconv.FormatUint(gapThrough, 10),
		Port:    PortGap,
		Payload: payload,
	}
}

// FormatCursor encodes a seqno as a source cursor.
func FormatCursor(seqno uint64) []byte {
	return []byte(strconv.FormatUint(seqno, 10))
}

// ParseCursor decodes a source cursor; an empty cursor is the inbox's start.
func ParseCursor(cursor []byte) (uint64, error) {
	if len(cursor) == 0 {
		return 0, nil
	}
	seqno, err := strconv.ParseUint(string(cursor), 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "parse cloud inbox cursor")
	}
	return seqno, nil
}

// _ is a type assertion
var _ flowgraph_eventsource.SRPCEventSourceServiceServer = (*Source)(nil)
