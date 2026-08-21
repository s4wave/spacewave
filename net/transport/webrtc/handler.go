package webrtc

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/scrub"
	"github.com/s4wave/spacewave/net/signaling"
)

// WebRTCSignalHandlerControllerID is the controller ID for WebRTCSignalHandler
const WebRTCSignalHandlerControllerID = ControllerID + "/signal-handler"

// WebRTCSignalHandler handles incoming signaling messages from other peers.
//
// This controller is usually started & managed by the WebRTC transport.
type WebRTCSignalHandler struct {
	t *WebRTC
}

// NewWebRTCSignalHandler constructs the WebRTCSignalHandler controller.
//
// Listens for HandleSignalPeer directives and calls the transport.
func NewWebRTCSignalHandler(t *WebRTC) *WebRTCSignalHandler {
	return &WebRTCSignalHandler{t: t}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
// It is safe to add a reference to the directive during this call.
// The passed context is canceled when the directive instance expires.
// NOTE: the passed context is not canceled when the handler is removed.
func (c *WebRTCSignalHandler) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case signaling.HandleSignalPeer:
		return c.resolveHandleSignalPeer(dir)
	}
	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *WebRTCSignalHandler) GetControllerInfo() *controller.Info {
	return controller.NewInfo(WebRTCSignalHandlerControllerID, Version, "webrtc incoming signal handler")
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *WebRTCSignalHandler) Execute(ctx context.Context) error {
	// no-op
	return nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *WebRTCSignalHandler) Close() error {
	// no-op
	return nil
}

// handleSignalPeerResolver resolves HandleSignalPeer.
type handleSignalPeerResolver struct {
	t    *WebRTC
	sess signaling.SignalPeerSession

	// closed reports that Resolve has exited. t.bcast guards closed.
	closed bool
}

// Resolve resolves the directive.
func (r *handleSignalPeerResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	remotePeerID := r.sess.GetRemotePeerID()
	remotePeerIDStr := remotePeerID.String()
	r.t.le.Debugf("started signaling session with %v", remotePeerIDStr)
	defer r.t.closeSignalIngress(remotePeerIDStr, r)

	for {
		// Wait for an incoming message.
		data, err := r.sess.Recv(ctx)
		if err != nil {
			return err
		}

		// The signature on the message was already verified.
		sig, err := DecodeWebRtcSignal(data, r.t.privKey)
		scrub.Scrub(data)
		if err == nil {
			err = sig.Validate()
		}
		if err != nil {
			r.t.le.WithError(err).Warnf("failed to decode incoming signal from %v", remotePeerIDStr)
			return err
		}
		incoming := &incomingSignal{
			sig:      sig,
			accepted: make(chan struct{}),
		}
		if err := r.t.deliverSignal(ctx, remotePeerIDStr, r, incoming); err != nil {
			return err
		}
	}
}

// resolveHandleSignalPeer resolves the HandleSignalPeer directive.
func (c *WebRTCSignalHandler) resolveHandleSignalPeer(dir signaling.HandleSignalPeer) ([]directive.Resolver, error) {
	// Check signaling id matches
	if dir.HandleSignalingID() != c.t.conf.GetSignalingId() {
		return nil, nil
	}

	// Check local peer ID matches
	localPeerID := dir.HandleSignalPeerSession().GetLocalPeerID()
	localPeerIDStr := localPeerID.String()
	actualLocalPeerIDStr := c.t.peerID.String()
	if localPeerIDStr != actualLocalPeerIDStr {
		c.t.le.Warnf("ignoring incoming signal for peer id %v: transport expects peer id %v", localPeerIDStr, actualLocalPeerIDStr)
		return nil, nil
	}

	// Check remote peer id is not blocked
	remotePeerIDStr := dir.HandleSignalPeerSession().GetRemotePeerID().String()
	if slices.Contains(c.t.conf.GetBlockPeers(), remotePeerIDStr) {
		return nil, nil
	}

	// Return resolver
	return directive.R(&handleSignalPeerResolver{
		t:    c.t,
		sess: dir.HandleSignalPeerSession(),
	}, nil)
}

// _ is a type assertion
var (
	_ controller.Controller = (*WebRTCSignalHandler)(nil)
	_ directive.Resolver    = (*handleSignalPeerResolver)(nil)
)
