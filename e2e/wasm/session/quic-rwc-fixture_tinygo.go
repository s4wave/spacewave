//go:build tinygo

package e2e_wasm_session

import (
	"context"

	"github.com/pkg/errors"
)

// ErrQuicRwcUnavailable reports the QUIC RWC fixture cannot run under tinygo.
// The manual signal transport behind the fixture uses pion/webrtc, which is
// excluded from tinygo builds.
var ErrQuicRwcUnavailable = errors.New("quic rwc fixture requires a non-tinygo compiler")

// RunQuicRwcFixture keeps the QUIC RWC fixture service registered under
// tinygo builds; the real browser WebRTC fixture excludes tinygo.
func (c *Controller) RunQuicRwcFixture(
	ctx context.Context,
	req *RunQuicRwcFixtureRequest,
) (*RunQuicRwcFixtureResponse, error) {
	return nil, ErrQuicRwcUnavailable
}
