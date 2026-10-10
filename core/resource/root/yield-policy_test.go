//go:build !js

package resource_root

import (
	"context"
	"testing"

	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// yieldPromptsStream records the responses of a WatchListenerYieldPrompts call.
type yieldPromptsStream struct {
	s4wave_root.SRPCRootResourceService_WatchListenerYieldPromptsStream
	// ctx is the stream context.
	ctx context.Context
	// sent receives each response.
	sent chan *s4wave_root.WatchListenerYieldPromptsResponse
}

// Context returns the stream context.
func (s *yieldPromptsStream) Context() context.Context { return s.ctx }

// Send records a response.
func (s *yieldPromptsStream) Send(resp *s4wave_root.WatchListenerYieldPromptsResponse) error {
	s.sent <- resp
	return nil
}

// runtimeHandoffStream records the responses of a WatchRuntimeHandoff call.
type runtimeHandoffStream struct {
	s4wave_root.SRPCRootResourceService_WatchRuntimeHandoffStream
	// ctx is the stream context.
	ctx context.Context
	// sent receives each response.
	sent chan *s4wave_root.WatchRuntimeHandoffResponse
}

// Context returns the stream context.
func (s *runtimeHandoffStream) Context() context.Context { return s.ctx }

// Send records a response.
func (s *runtimeHandoffStream) Send(resp *s4wave_root.WatchRuntimeHandoffResponse) error {
	s.sent <- resp
	return nil
}

// TestYieldWatchesWithoutBroker verifies that a runtime without a resource
// listener answers the yield watches with one empty snapshot and then waits,
// instead of failing a call the app retries without pause.
func TestYieldWatchesWithoutBroker(t *testing.T) {
	// Watch the prompts of a server with no yield broker.
	server := newCloseTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	prompts := &yieldPromptsStream{ctx: ctx, sent: make(chan *s4wave_root.WatchListenerYieldPromptsResponse, 1)}
	handoff := &runtimeHandoffStream{ctx: ctx, sent: make(chan *s4wave_root.WatchRuntimeHandoffResponse, 1)}
	promptsErr := make(chan error, 1)
	handoffErr := make(chan error, 1)
	go func() {
		promptsErr <- server.WatchListenerYieldPrompts(&s4wave_root.WatchListenerYieldPromptsRequest{}, prompts)
	}()
	go func() {
		handoffErr <- server.WatchRuntimeHandoff(&s4wave_root.WatchRuntimeHandoffRequest{}, handoff)
	}()

	// Expect one empty snapshot from each watch.
	if resp := <-prompts.sent; len(resp.GetPrompts()) != 0 {
		t.Fatalf("unexpected prompts: %v", resp.GetPrompts())
	}
	if resp := <-handoff.sent; resp.GetState().GetActive() {
		t.Fatalf("unexpected active handoff: %v", resp.GetState())
	}

	// Expect both watches to end cleanly with the stream.
	cancel()
	if err := <-promptsErr; err != nil {
		t.Fatal(err)
	}
	if err := <-handoffErr; err != nil {
		t.Fatal(err)
	}
}
