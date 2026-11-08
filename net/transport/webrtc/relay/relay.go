// Package webrtc_relay hands a signed-in session short-lived TURN credentials from
// the app server holding the provider account.
//
// The server answers each session through an Issuer; the provider token stays
// in the server's configuration. The client runs a Controller for its session
// peer, which resolves LookupICEProvider with credentials fetched for each new
// connection and keeps none of them.
package webrtc_relay

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/webrtc"
)

// MinRemaining is the shortest remaining lifetime a new link starts on.
const MinRemaining = time.Minute

// ErrUnavailable is returned by an Issuer with no provider configured.
var ErrUnavailable = errors.New("relay credentials unavailable")

// ErrExpiring is returned when fetched credentials expire within MinRemaining.
var ErrExpiring = errors.New("relay credentials expire too soon")

// Issuer issues relay credentials on the server holding the provider account.
type Issuer interface {
	// Issue returns fresh credentials for the signed-in session peer.
	// Returns ErrUnavailable when no provider is configured.
	Issue(ctx context.Context, session peer.ID) (*Credentials, error)
}

// Fetch requests fresh credentials for the session from the app server.
type Fetch func(ctx context.Context) (*Credentials, error)

// RelayServers returns copies of the relay servers in c that carry a
// credential, with only their turn: and turns: URLs. STUN URLs and servers
// without a credential are dropped: the session adds its STUN servers from
// its own config.
func (c *Credentials) RelayServers() []*webrtc.IceServerConfig {
	var out []*webrtc.IceServerConfig
	for _, srv := range c.GetIceServers() {
		// Keep only servers that carry a credential.
		if srv.GetUsername() == "" || srv.GetCredential() == nil {
			continue
		}

		// Keep only their relay URLs.
		var urls []string
		for _, u := range srv.GetUrls() {
			if strings.HasPrefix(u, "turn:") || strings.HasPrefix(u, "turns:") {
				urls = append(urls, u)
			}
		}
		if len(urls) == 0 {
			continue
		}
		relay := srv.CloneVT()
		relay.Urls = urls
		out = append(out, relay)
	}
	return out
}

// Remaining returns the credentials' lifetime left at now, or zero when they
// carry no expiry.
func (c *Credentials) Remaining(now time.Time) time.Duration {
	exp := c.GetExpiresAt()
	if exp == nil {
		return 0
	}
	return exp.AsTime().Sub(now)
}
