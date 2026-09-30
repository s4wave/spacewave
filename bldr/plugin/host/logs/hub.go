package plugin_host_logs

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/broadcast"
)

// defaultRetainedEventLimit is the retained event count when unset.
const defaultRetainedEventLimit uint32 = 1000

// Hub owns structured plugin log events for a host process.
type Hub struct {
	bcast broadcast.Broadcast

	now                func() time.Time
	retainedEventLimit uint32
	nextSequence       uint64
	retained           []*StructuredLogEvent

	views           map[*View]struct{}
	followViewCount uint64
}

// HubOption configures a Hub.
type HubOption func(*Hub)

// WithRetainedEventLimit sets the maximum number of retained events.
func WithRetainedEventLimit(limit uint32) HubOption {
	// Store the limit on the hub.
	return func(h *Hub) {
		h.retainedEventLimit = limit
	}
}

// WithClock sets the clock Hub reads when it timestamps an event.
func WithClock(now func() time.Time) HubOption {
	// Store the clock on the hub.
	return func(h *Hub) {
		h.now = now
	}
}

// NewHub constructs a structured log hub.
func NewHub(opts ...HubOption) *Hub {
	// Build the hub with defaults and apply the options.
	h := &Hub{
		now:                time.Now,
		retainedEventLimit: defaultRetainedEventLimit,
		views:              make(map[*View]struct{}),
	}
	for _, opt := range opts {
		opt(h)
	}

	// Fall back to the wall clock when no clock was configured.
	if h.now == nil {
		h.now = time.Now
	}
	return h
}

// Emit appends an event and fills in the metadata Hub assigns.
func (h *Hub) Emit(event *StructuredLogEvent) (*EmitStructuredLogResponse, error) {
	// A nil event is rejected.
	if event == nil {
		return nil, errors.New("structured log event cannot be nil")
	}

	// Take the hub lock for the emit.
	locked := h.bcast.Lock()
	defer locked.Unlock()

	// Assign the sequence number and timestamp to the event.
	h.nextSequence++
	assigned := event.CloneVT()
	assigned.Sequence = h.nextSequence
	assigned.Timestamp = timestamppb.New(h.now())

	// Retain the event while views are open and retention is enabled.
	if h.retainedEventLimit != 0 && len(h.views) != 0 {
		h.retained = append(h.retained, assigned)
		if uint32(len(h.retained)) > h.retainedEventLimit { //nolint:gosec // retained is bounded by the uint32 event limit before this comparison.
			copy(h.retained, h.retained[1:])
			h.retained[len(h.retained)-1] = nil
			h.retained = h.retained[:len(h.retained)-1]
		}
	}

	// Update every following view that matches the event.
	if h.followViewCount != 0 {
		for view := range h.views {
			if !view.rangeSnapshot.GetFollow() {
				continue
			}
			if !matchesFilter(assigned, view.filter) {
				continue
			}
			if h.retainedEventLimit == 0 {
				if view.appendFollowEventLocked(assigned) {
					view.notifyLocked()
				}
				continue
			}
			view.state = h.buildStateLocked(view.filter, view.rangeSnapshot)
			view.notifyLocked()
		}
	}

	// Publish the change and return the assigned metadata.
	locked.Broadcast()
	return &EmitStructuredLogResponse{
		Sequence:  assigned.Sequence,
		Timestamp: assigned.Timestamp.CloneVT(),
	}, nil
}

// Snapshot returns the current retained state for a filter and range.
func (h *Hub) Snapshot(filter *StructuredLogFilter, rng *StructuredLogRange) *StructuredLogState {
	locked := h.bcast.Lock()
	defer locked.Unlock()

	// Build the state snapshot under the lock.
	return h.buildStateLocked(filter, rng)
}

// OpenView opens a mutable structured-log view.
func (h *Hub) OpenView(filter *StructuredLogFilter, rng *StructuredLogRange) *View {
	// Take the hub lock for the open.
	locked := h.bcast.Lock()
	defer locked.Unlock()

	// Build the view and its initial state.
	view := &View{
		hub:           h,
		filter:        filter.CloneVT(),
		rangeSnapshot: rng.CloneVT(),
		updates:       newViewUpdateSignal(),
	}
	view.state = h.buildStateLocked(view.filter, view.rangeSnapshot)

	// Register the view and track following views.
	h.views[view] = struct{}{}
	if view.rangeSnapshot.GetFollow() {
		h.followViewCount++
	}
	locked.Broadcast()
	return view
}

// buildStateLocked builds a log state snapshot from the retained events,
// applying the filter and range. Caller must hold the Hub lock.
func (h *Hub) buildStateLocked(filter *StructuredLogFilter, rng *StructuredLogRange) *StructuredLogState {
	// Clone the filter and range for the snapshot.
	filter = filter.CloneVT()
	rng = rng.CloneVT()

	// Collect the retained events matching the filter and range.
	var matches []*StructuredLogEvent
	var skipped uint64
	after := rng.GetAfterSequence()
	for _, event := range h.retained {
		if !matchesFilter(event, filter) {
			continue
		}
		if after != 0 && event.GetSequence() <= after {
			skipped++
			continue
		}
		matches = append(matches, event)
	}

	// Apply the range limit from the head or tail.
	limit := int(rng.GetLimit())
	if rng.GetTail() && limit > 0 && len(matches) > limit {
		skipped += uint64(len(matches) - limit) //nolint:gosec // both values are non-negative lengths after the positive limit and len check.
		matches = matches[len(matches)-limit:]
	} else if !rng.GetTail() && limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	// Clone the matched events into the returned state.
	events := make([]*StructuredLogEvent, len(matches))
	for i, event := range matches {
		events[i] = event.CloneVT()
	}

	return &StructuredLogState{
		Filter:            filter,
		Range:             rng,
		Events:            events,
		DroppedEventCount: skipped,
	}
}

// View is an open structured-log view.
type View struct {
	hub *Hub

	filter        *StructuredLogFilter
	rangeSnapshot *StructuredLogRange
	state         *StructuredLogState
	updates       *viewUpdateSignal
	released      bool
}

// Updates returns a channel signaled when the view state changes.
func (v *View) Updates() <-chan struct{} {
	return v.updates.Updates()
}

// Snapshot returns the view's current state.
func (v *View) Snapshot() *StructuredLogState {
	locked := v.hub.bcast.Lock()
	defer locked.Unlock()

	return v.state.CloneVT()
}

// Set updates the view filter and range and returns the new state.
func (v *View) Set(filter *StructuredLogFilter, rng *StructuredLogRange) *StructuredLogState {
	// Take the hub lock for the update.
	locked := v.hub.bcast.Lock()
	defer locked.Unlock()

	// Store the new filter and range.
	wasFollow := v.rangeSnapshot.GetFollow()
	v.filter = filter.CloneVT()
	v.rangeSnapshot = rng.CloneVT()
	isFollow := v.rangeSnapshot.GetFollow()

	// Track the follow view count across the change.
	if !v.released && wasFollow != isFollow {
		if isFollow {
			v.hub.followViewCount++
		} else {
			v.hub.followViewCount--
		}
	}

	// Rebuild the state and notify followers.
	v.state = v.hub.buildStateLocked(v.filter, v.rangeSnapshot)
	v.notifyLocked()
	locked.Broadcast()
	return v.state.CloneVT()
}

// Release closes the view.
func (v *View) Release() {
	// Take the hub lock for the release.
	locked := v.hub.bcast.Lock()
	defer locked.Unlock()

	// Release an already released view only once.
	if v.released {
		return
	}
	v.released = true

	// Drop the view and its follow count.
	delete(v.hub.views, v)
	if v.rangeSnapshot.GetFollow() {
		v.hub.followViewCount--
	}

	// Drop the retained events when the last view closes.
	if len(v.hub.views) == 0 {
		clear(v.hub.retained)
		v.hub.retained = nil
	}

	// Close the update signal and publish the change.
	v.updates.Close()
	locked.Broadcast()
}

// notifyLocked signals waiting followers. Caller must hold the View lock.
func (v *View) notifyLocked() {
	v.updates.Notify()
}

// viewUpdateSignal is a broadcast-backed change signal for one View.
type viewUpdateSignal struct {
	bcast  broadcast.Broadcast
	ch     chan struct{}
	closed bool
}

// newViewUpdateSignal constructs an empty update signal.
func newViewUpdateSignal() *viewUpdateSignal {
	return &viewUpdateSignal{ch: make(chan struct{}, 1)}
}

// Updates returns the change-notification channel.
func (s *viewUpdateSignal) Updates() <-chan struct{} {
	locked := s.bcast.Lock()
	defer locked.Unlock()

	// Return the channel under the lock.
	return s.ch
}

// Notify signals one waiting follower.
func (s *viewUpdateSignal) Notify() {
	// Take the signal lock for the notification.
	locked := s.bcast.Lock()
	defer locked.Unlock()

	// A closed signal drops notifications.
	if s.closed {
		return
	}

	// Signal without blocking and publish the change.
	select {
	case s.ch <- struct{}{}:
	default:
	}
	locked.Broadcast()
}

// Close closes the signal channel.
func (s *viewUpdateSignal) Close() {
	// Take the signal lock for the close.
	locked := s.bcast.Lock()
	defer locked.Unlock()

	// Close the signal only once.
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
	locked.Broadcast()
}

// appendFollowEventLocked appends an event to a following View's state,
// returning false when the event falls outside the follow range. Caller
// must hold the View lock.
func (v *View) appendFollowEventLocked(event *StructuredLogEvent) bool {
	// Events at or before the range start fall outside the follow range.
	if event.GetSequence() <= v.rangeSnapshot.GetAfterSequence() {
		return false
	}

	// Apply the follow limit from the head or tail.
	events := append(v.state.GetEvents(), event.CloneVT())
	limit := int(v.rangeSnapshot.GetLimit())
	if limit > 0 && len(events) > limit {
		if !v.rangeSnapshot.GetTail() {
			return false
		}
		dropped := len(events) - limit
		v.state.DroppedEventCount += uint64(dropped) //nolint:gosec // dropped is the non-negative length removed from the bounded event slice.
		events = events[dropped:]
	}

	// Store the updated events.
	v.state.Events = events
	return true
}

func matchesFilter(event *StructuredLogEvent, filter *StructuredLogFilter) bool {
	// A nil filter matches everything.
	if filter == nil {
		return true
	}

	// Check the plugin, instance, stream, and level filters.
	if pluginIDs := filter.GetPluginIds(); len(pluginIDs) != 0 && !slices.Contains(pluginIDs, event.GetPluginId()) {
		return false
	}
	if instanceKeys := filter.GetInstanceKeys(); len(instanceKeys) != 0 && !slices.Contains(instanceKeys, event.GetInstanceKey()) {
		return false
	}
	if streams := filter.GetStreams(); len(streams) != 0 && !slices.Contains(streams, event.GetStream()) {
		return false
	}
	if minLevel := filter.GetMinLevel(); minLevel != StructuredLogLevel_STRUCTURED_LOG_LEVEL_UNSPECIFIED && event.GetLevel() < minLevel {
		return false
	}

	// Check the field values and the search text.
	for key, value := range filter.GetFields() {
		if event.GetFields()[key] != value {
			return false
		}
	}
	if searchText := strings.ToLower(filter.GetSearchText()); searchText != "" && !eventContains(event, searchText) {
		return false
	}
	return true
}

func eventContains(event *StructuredLogEvent, searchText string) bool {
	// Check the message text.
	if strings.Contains(strings.ToLower(event.GetMessage()), searchText) {
		return true
	}

	// Check each field key and value.
	for key, value := range event.GetFields() {
		if strings.Contains(strings.ToLower(key), searchText) || strings.Contains(strings.ToLower(value), searchText) {
			return true
		}
	}
	return false
}
