//go:build !js

package spacewave_cli

import (
	"context"
	"io"

	"github.com/aperturerobotics/starpc/srpc"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
)

// failingPresenceClient supplies an active generation whose stream later fails.
type failingPresenceClient struct {
	bldr_web_plugin.SRPCWebPluginClient
	// openErr fails before the first presence receive.
	openErr error
	// stream is the selected generation's presence stream.
	stream *failingPresenceStream
	// exit carries the owner's terminal state when configured.
	exit <-chan struct{}
	// waitStarted reports that the daemon requested the owner wait.
	waitStarted chan<- struct{}
	// waitGate delays the owner wait until after shell exit in the late-call fixture.
	waitGate <-chan struct{}
}

// SRPCClient identifies this fake plugin instance for generation comparison.
func (c *failingPresenceClient) SRPCClient() srpc.Client { return nil }

// WatchDesktopPresence returns the generation selected for the failure test.
func (c *failingPresenceClient) WatchDesktopPresence(ctx context.Context, _ *bldr_web_plugin.WatchDesktopPresenceRequest) (bldr_web_plugin.SRPCWebPlugin_WatchDesktopPresenceClient, error) {
	// Return the prepared presence stream, or the open error.
	if c.openErr != nil {
		return nil, c.openErr
	}
	stream := *c.stream
	stream.ctx = ctx
	stream.first = false
	return &stream, nil
}

// WaitDesktopExit returns the retained terminal state after the owner exits.
func (c *failingPresenceClient) WaitDesktopExit(ctx context.Context, _ *bldr_web_plugin.WatchDesktopPresenceRequest) (*bldr_web_plugin.WatchDesktopPresenceResponse, error) {
	// Expose the entered wait and hold it until the fixture permits owner readback.
	if c.waitStarted != nil {
		close(c.waitStarted)
	}
	if c.waitGate != nil {
		select {
		case <-c.waitGate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Return the terminal owner event whenever it already exists or later arrives.
	select {
	case <-c.exit:
		return &bldr_web_plugin.WatchDesktopPresenceResponse{State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// OpenOrFocusDesktop acknowledges the shell before its observation fails.
func (c *failingPresenceClient) OpenOrFocusDesktop(context.Context, *bldr_web_plugin.OpenOrFocusDesktopRequest) (*bldr_web_plugin.OpenOrFocusDesktopResponse, error) {
	return &bldr_web_plugin.OpenOrFocusDesktopResponse{Generation: 1}, nil
}

// failingPresenceStream reports ACTIVE once and then loses its plugin stream.
type failingPresenceStream struct {
	srpc.Stream
	// ctx ends the receive when the daemon stops.
	ctx context.Context
	// failFirst simulates losing presence before any owner confirmation.
	failFirst bool
	// fail releases the next receive with a stream error.
	fail <-chan struct{}
	// first records whether ACTIVE was already emitted.
	first bool
}

// Recv reports the shell's first state before the stream fails.
func (s *failingPresenceStream) Recv() (*bldr_web_plugin.WatchDesktopPresenceResponse, error) {
	if !s.first && !s.failFirst {
		s.first = true
		return &bldr_web_plugin.WatchDesktopPresenceResponse{State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE}, nil
	}
	select {
	case <-s.fail:
		return nil, io.EOF
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

// Close releases this test stream after the daemon drops demand.
func (s *failingPresenceStream) Close() error { return nil }

// RecvTo satisfies the generated stream contract.
func (s *failingPresenceStream) RecvTo(dst *bldr_web_plugin.WatchDesktopPresenceResponse) error {
	state, err := s.Recv()
	if err != nil {
		return err
	}
	*dst = *state
	return nil
}

// _ is a type assertion.
var _ bldr_web_plugin.SRPCWebPlugin_WatchDesktopPresenceClient = (*failingPresenceStream)(nil)
