//go:build !js

package wasm

import (
	"testing"

	"github.com/pkg/errors"
)

func TestBrowserPeerErrorClassification(t *testing.T) {
	deadlineErr := errors.New("context deadline exceeded")
	if !isBrowserPeerStartupErr(deadlineErr) {
		t.Fatal("expected deadline errors to be treated as startup races")
	}
	if shouldAbandonBrowserPeer(deadlineErr) {
		t.Fatal("expected deadline errors to keep retrying the current peer")
	}

	closedErr := errors.New("resource client: quic: transport closed")
	if !shouldAbandonBrowserPeer(closedErr) {
		t.Fatal("expected closed transports to abandon the peer")
	}
}
