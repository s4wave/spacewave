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
}

// SRPCClient identifies this fake plugin instance for generation comparison.
func (c *failingPresenceClient) SRPCClient() srpc.Client { return nil }

// WatchDesktopPresence returns the generation selected for the failure test.
func (c *failingPresenceClient) WatchDesktopPresence(ctx context.Context, _ *bldr_web_plugin.WatchDesktopPresenceRequest) (bldr_web_plugin.SRPCWebPlugin_WatchDesktopPresenceClient, error) {
	if c.openErr != nil {
		return nil, c.openErr
	}
	c.stream.ctx = ctx
	return c.stream, nil
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
