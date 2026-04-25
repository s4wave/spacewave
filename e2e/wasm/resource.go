//go:build !js

package wasm

import (
	"context"
	"strings"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	devtool_web "github.com/s4wave/spacewave/bldr/devtool/web"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/net/peer"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"github.com/sirupsen/logrus"
)

// browserProtocolID is the protocol ID for devtool-to-browser RPC streams.
var browserProtocolID = devtool_web.BrowserProtocolID

// connectSessionResources connects a Resource SDK client on the given
// TestSession through the bifrost link between the devtool bus and the
// browser context's WASM process.
func (h *Harness) connectSessionResources(ctx context.Context, s *TestSession) error {
	le := logrus.WithField("component", "harness")

	var lastErr error
	attemptTimeout := 10 * time.Second
	maxBackoff := 5 * time.Second
	maxStartupRetry := 15 * time.Second

	for {
		le.Info("waiting for new browser peer")
		browserPeer, err := h.getPeerWatcher().WaitForNewPeer(ctx)
		if err != nil {
			if lastErr != nil {
				return errors.Wrap(lastErr, "connect resources (last attempt)")
			}
			return errors.Wrap(err, "discover browser peer")
		}
		le.WithField("peer", browserPeer.String()).Info("discovered browser peer")
		if !h.leaseBrowserPeer(s, browserPeer) {
			le.WithField("peer", browserPeer.String()).Info("browser peer already leased to another session, waiting for another")
			continue
		}

		backoff := time.Second
		peerStart := time.Now()
		for {
			attemptCtx, attemptCancel := context.WithTimeout(ctx, attemptTimeout)
			err := h.tryConnectSession(attemptCtx, s, browserPeer)
			attemptCancel()
			if err == nil {
				s.browserPeer = browserPeer
				return nil
			}

			lastErr = err
			entry := le.WithField("peer", browserPeer.String()).WithError(err)
			if isBrowserPeerStartupErr(err) && time.Since(peerStart) < maxStartupRetry {
				entry.Info("resource connection hit startup race, retrying same peer")
			} else if shouldAbandonBrowserPeer(err) {
				h.releaseBrowserPeerLease(s, browserPeer)
				entry.Info("resource connection failed on stale browser peer, waiting for another")
				break
			} else {
				entry.Info("resource connection attempt failed, retrying")
			}

			select {
			case <-ctx.Done():
				h.releaseBrowserPeerLease(s, browserPeer)
				return errors.Wrap(lastErr, "connect resources (last attempt)")
			case <-time.After(backoff):
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// tryConnectSession attempts a single resource connection on the TestSession.
func (h *Harness) tryConnectSession(ctx context.Context, s *TestSession, browserPeer peer.ID) error {
	openStreamFn := stream_srpc.NewOpenStreamFunc(
		h.devtool.GetBus(),
		browserProtocolID,
		peer.ID(""), // any local peer
		browserPeer,
		0, // any transport
	)
	client := srpc.NewClient(openStreamFn)

	serviceID := "plugin/spacewave-core/" + resource.SRPCResourceServiceServiceID
	resourceSvc := resource.NewSRPCResourceServiceClientWithServiceID(client, serviceID)
	resClient, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		return errors.Wrap(err, "resource client")
	}

	rootRef := resClient.AccessRootResource()
	root, err := s4wave_root.NewRoot(resClient, rootRef)
	if err != nil {
		rootRef.Release()
		resClient.Release()
		return errors.Wrap(err, "root resource")
	}

	s.browserClient = client
	s.resClient = resClient
	s.root = root
	return nil
}

func shouldAbandonBrowserPeer(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "transport closed") ||
		strings.Contains(msg, "failed to get reader") ||
		strings.Contains(msg, "StatusGoingAway") ||
		strings.Contains(msg, "ERR_STREAM_IDLE") ||
		strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "context deadline exceeded")
}

func isBrowserPeerStartupErr(err error) bool {
	return strings.Contains(err.Error(), "disconnected before registering")
}

// getPeerWatcher returns the shared PeerWatcher, creating it on the first
// call. The PeerWatcher tracks browser peers across sessions.
func (h *Harness) getPeerWatcher() *PeerWatcher {
	h.peerWatcherOnce.Do(func() {
		pw, err := NewPeerWatcher(h.devtool.GetBus())
		if err != nil {
			panic("peer watcher: " + err.Error())
		}
		h.peerWatcher = pw
	})
	return h.peerWatcher
}
