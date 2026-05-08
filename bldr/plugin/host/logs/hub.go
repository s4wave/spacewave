package plugin_host_logs

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

const defaultRetainedEventLimit uint32 = 1000

// Hub owns structured plugin log events for a host process.
type Hub struct {
	mu sync.Mutex

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
	return func(h *Hub) {
		h.retainedEventLimit = limit
	}
}

// WithClock sets the clock used for owner-assigned event timestamps.
func WithClock(now func() time.Time) HubOption {
	return func(h *Hub) {
		h.now = now
	}
}

// NewHub constructs a structured log hub.
func NewHub(opts ...HubOption) *Hub {
	h := &Hub{
		now:                time.Now,
		retainedEventLimit: defaultRetainedEventLimit,
		views:              make(map[*View]struct{}),
	}
	for _, opt := range opts {
		opt(h)
	}
	if h.now == nil {
		h.now = time.Now
	}
	return h
}

// Emit appends an event and assigns owner-controlled metadata.
func (h *Hub) Emit(event *StructuredLogEvent) (*EmitStructuredLogResponse, error) {
	if event == nil {
		return nil, errors.New("structured log event cannot be nil")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.nextSequence++
	assigned := event.CloneVT()
	assigned.Sequence = h.nextSequence
	assigned.Timestamp = timestamppb.New(h.now())

	if h.retainedEventLimit != 0 {
		h.retained = append(h.retained, assigned)
		if uint32(len(h.retained)) > h.retainedEventLimit {
			copy(h.retained, h.retained[1:])
			h.retained[len(h.retained)-1] = nil
			h.retained = h.retained[:len(h.retained)-1]
		}
	}

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

	return &EmitStructuredLogResponse{
		Sequence:  assigned.Sequence,
		Timestamp: assigned.Timestamp.CloneVT(),
	}, nil
}

// Snapshot returns the current retained state for a filter and range.
func (h *Hub) Snapshot(filter *StructuredLogFilter, rng *StructuredLogRange) *StructuredLogState {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.buildStateLocked(filter, rng)
}

// OpenView opens a mutable structured-log view.
func (h *Hub) OpenView(filter *StructuredLogFilter, rng *StructuredLogRange) *View {
	h.mu.Lock()
	defer h.mu.Unlock()

	view := &View{
		hub:           h,
		filter:        filter.CloneVT(),
		rangeSnapshot: rng.CloneVT(),
		updates:       make(chan struct{}, 1),
	}
	view.state = h.buildStateLocked(view.filter, view.rangeSnapshot)
	h.views[view] = struct{}{}
	if view.rangeSnapshot.GetFollow() {
		h.followViewCount++
	}
	return view
}

func (h *Hub) buildStateLocked(filter *StructuredLogFilter, rng *StructuredLogRange) *StructuredLogState {
	filter = filter.CloneVT()
	rng = rng.CloneVT()

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

	limit := int(rng.GetLimit())
	if rng.GetTail() && limit > 0 && len(matches) > limit {
		skipped += uint64(len(matches) - limit)
		matches = matches[len(matches)-limit:]
	} else if !rng.GetTail() && limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

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
	updates       chan struct{}
	released      bool
}

// Updates returns a channel signaled when the view state changes.
func (v *View) Updates() <-chan struct{} {
	return v.updates
}

// Snapshot returns the view's current state.
func (v *View) Snapshot() *StructuredLogState {
	v.hub.mu.Lock()
	defer v.hub.mu.Unlock()

	return v.state.CloneVT()
}

// Set updates the view filter and range and returns the new state.
func (v *View) Set(filter *StructuredLogFilter, rng *StructuredLogRange) *StructuredLogState {
	v.hub.mu.Lock()
	defer v.hub.mu.Unlock()

	wasFollow := v.rangeSnapshot.GetFollow()
	v.filter = filter.CloneVT()
	v.rangeSnapshot = rng.CloneVT()
	isFollow := v.rangeSnapshot.GetFollow()
	if !v.released && wasFollow != isFollow {
		if isFollow {
			v.hub.followViewCount++
		} else {
			v.hub.followViewCount--
		}
	}
	v.state = v.hub.buildStateLocked(v.filter, v.rangeSnapshot)
	v.notifyLocked()
	return v.state.CloneVT()
}

// Release closes the view.
func (v *View) Release() {
	v.hub.mu.Lock()
	defer v.hub.mu.Unlock()

	if v.released {
		return
	}
	v.released = true
	delete(v.hub.views, v)
	if v.rangeSnapshot.GetFollow() {
		v.hub.followViewCount--
	}
	close(v.updates)
}

func (v *View) notifyLocked() {
	select {
	case v.updates <- struct{}{}:
	default:
	}
}

func (v *View) appendFollowEventLocked(event *StructuredLogEvent) bool {
	if event.GetSequence() <= v.rangeSnapshot.GetAfterSequence() {
		return false
	}

	events := append(v.state.GetEvents(), event.CloneVT())
	limit := int(v.rangeSnapshot.GetLimit())
	if limit > 0 && len(events) > limit {
		if !v.rangeSnapshot.GetTail() {
			return false
		}
		dropped := len(events) - limit
		v.state.DroppedEventCount += uint64(dropped)
		events = events[dropped:]
	}

	v.state.Events = events
	return true
}

func matchesFilter(event *StructuredLogEvent, filter *StructuredLogFilter) bool {
	if filter == nil {
		return true
	}
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
	if strings.Contains(strings.ToLower(event.GetMessage()), searchText) {
		return true
	}
	for key, value := range event.GetFields() {
		if strings.Contains(strings.ToLower(key), searchText) || strings.Contains(strings.ToLower(value), searchText) {
			return true
		}
	}
	return false
}
