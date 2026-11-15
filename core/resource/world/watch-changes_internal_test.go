//go:build !js

package resource_world

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// watchChangesStream is an in-process WatchChanges stream: Recv returns the
// queued requests, then io.EOF once they are closed, or a queued error.
type watchChangesStream struct {
	srpc.Stream
	ctx  context.Context
	reqs chan any
}

func (s *watchChangesStream) Context() context.Context { return s.ctx }

func (s *watchChangesStream) Send(*s4wave_world.WatchChangesResponse) error { return nil }

func (s *watchChangesStream) SendAndClose(*s4wave_world.WatchChangesResponse) error { return nil }

func (s *watchChangesStream) Recv() (*s4wave_world.WatchChangesRequest, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case req, ok := <-s.reqs:
		if !ok {
			return nil, io.EOF
		}
		if err, isErr := req.(error); isErr {
			return nil, err
		}
		return req.(*s4wave_world.WatchChangesRequest), nil
	}
}

func (s *watchChangesStream) RecvTo(*s4wave_world.WatchChangesRequest) error {
	return errors.New("watchChangesStream: RecvTo is not used")
}

// TestWatchChangesRefusesRequest checks the error a WatchChanges stream ends
// with: ErrInterestVersion for a request that should not have come, and a
// failed read's own error.
func TestWatchChangesRefusesRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	r := &EngineResource{engine: tb.Engine}
	errRead := errors.New("read failed")
	named := &s4wave_world.ChangeInterestDelta{Version: 1, AddKeys: []string{"refuse/a"}}
	cases := []struct {
		name string
		reqs []any
		want error
	}{
		{"first version 2", []any{&s4wave_world.WatchChangesRequest{Interest: &s4wave_world.ChangeInterestDelta{Version: 2}}}, ErrInterestVersion},
		{"delta without interest", []any{&s4wave_world.WatchChangesRequest{}, &s4wave_world.WatchChangesRequest{Interest: named}}, ErrInterestVersion},
		{"delta out of order", []any{&s4wave_world.WatchChangesRequest{Interest: named}, &s4wave_world.WatchChangesRequest{Interest: &s4wave_world.ChangeInterestDelta{Version: 3}}}, ErrInterestVersion},
		{"read failed without interest", []any{&s4wave_world.WatchChangesRequest{}, errRead}, errRead},
		{"read failed with interest", []any{&s4wave_world.WatchChangesRequest{Interest: named}, errRead}, errRead},
	}
	for _, c := range cases {
		reqs := make(chan any, len(c.reqs))
		for _, req := range c.reqs {
			reqs <- req
		}
		err := r.WatchChanges(&watchChangesStream{ctx: ctx, reqs: reqs})
		if !errors.Is(err, c.want) {
			t.Fatalf("%s: stream ended with %v, want %v", c.name, err, c.want)
		}
	}
}
