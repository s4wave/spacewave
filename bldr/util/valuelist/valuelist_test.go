package valuelist

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
)

// testResponse implements WatchDirectiveResponse for tests.
type testResponse struct {
	valueID uint32
	idle    uint32
	removed bool
	value   string
}

func (r *testResponse) GetValueId() uint32      { return r.valueID }
func (r *testResponse) SetValueId(id uint32)    { r.valueID = id }
func (r *testResponse) GetIdle() uint32         { return r.idle }
func (r *testResponse) SetIdle(idle uint32)     { r.idle = idle }
func (r *testResponse) GetRemoved() bool        { return r.removed }
func (r *testResponse) SetRemoved(removed bool) { r.removed = removed }
func (r *testResponse) GetValue() string        { return r.value }
func (r *testResponse) SetValue(val string)     { r.value = val }

// testStream returns queued responses from Recv.
type testStream struct {
	srpc.StreamRecv[*testResponse]
	msgs []*testResponse
}

func (s *testStream) Recv() (*testResponse, error) {
	if len(s.msgs) == 0 {
		return nil, io.EOF
	}
	msg := s.msgs[0]
	s.msgs = s.msgs[1:]
	return msg, nil
}

func (s *testStream) CloseSend() error { return nil }
func (s *testStream) Close() error     { return nil }

// testHandler assigns local value IDs starting at 100.
type testHandler struct {
	directive.ValueHandler
	nextID uint32
	values map[uint32]directive.Value
}

func (h *testHandler) AddValue(val directive.Value) (uint32, bool) {
	h.nextID++
	id := h.nextID + 100
	h.values[id] = val
	return id, true
}

func (h *testHandler) RemoveValue(id uint32) (directive.Value, bool) {
	val, ok := h.values[id]
	delete(h.values, id)
	return val, ok
}

// TestWatchDirectiveViaStreamRemoveMapsIDs checks removals use the local ID
// returned by AddValue instead of the remote value ID.
func TestWatchDirectiveViaStreamRemoveMapsIDs(t *testing.T) {
	strm := &testStream{msgs: []*testResponse{
		{valueID: 1, value: "a"},
		{valueID: 2, value: "b"},
		{valueID: 1, removed: true},
		{idle: 2},
	}}
	hnd := &testHandler{values: make(map[uint32]directive.Value)}
	err := WatchDirectiveViaStream[string, *testResponse](
		context.Background(),
		strm,
		hnd,
		nil,
		true,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(hnd.values) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(hnd.values), hnd.values)
	}
	for _, val := range hnd.values {
		if val != "b" {
			t.Fatalf("expected remaining value b, got %v", val)
		}
	}
}
