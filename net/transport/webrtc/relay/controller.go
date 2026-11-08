package webrtc_relay

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/webrtc"
)

// ControllerID is the session relay controller ID.
const ControllerID = "webrtc/relay/session"

// Version is the version of the controller.
var Version = controller.MustParseVersion("0.0.1")

// Controller resolves LookupICEProvider for one session peer with relay
// credentials fetched from the app server for each new connection. It keeps
// no credential between calls, so an established link keeps the credentials
// it started with and nothing outlives the attempt that used it.
type Controller struct {
	// session is the only local peer the controller answers for.
	session peer.ID
	// fetch requests credentials from the app server.
	fetch Fetch
	// base holds the session's STUN servers and transport policy.
	base *webrtc.WebRtcConfig
	// now returns the current time.
	now func() time.Time
}

// NewController constructs the relay controller for the session peer, fetching
// credentials with fetch. base, which may be nil, supplies the STUN servers and
// transport policy combined with each fetch's relays, since a resolved provider
// replaces the transport's static config.
func NewController(session peer.ID, fetch Fetch, base *webrtc.WebRtcConfig) *Controller {
	return &Controller{session: session, fetch: fetch, base: base, now: time.Now}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "session relay credentials")
}

// Execute executes the controller.
func (c *Controller) Execute(context.Context) error { return nil }

// HandleDirective resolves LookupICEProvider for the session peer only, so a
// plugin or another session on the bus cannot draw on this session's account.
func (c *Controller) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	d, ok := di.GetDirective().(webrtc.LookupICEProvider)
	if !ok || d.ICEPeerID() != c.session {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]webrtc.ICEProvider{c}), nil)
}

// ICEConfig fetches credentials for one connection attempt and returns the
// base config with their relays. Credentials expiring within MinRemaining are
// refused and fetched once more.
func (c *Controller) ICEConfig(ctx context.Context) (*webrtc.WebRtcConfig, error) {
	// Fetch credentials with enough lifetime left to start a link.
	var creds *Credentials
	for range 2 {
		var err error
		creds, err = c.fetch(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "fetch relay credentials")
		}
		if creds.Remaining(c.now()) >= MinRemaining {
			break
		}
		creds = nil
	}
	if creds == nil {
		return nil, ErrExpiring
	}

	// Add their relays to the session's own servers.
	relays := creds.RelayServers()
	if len(relays) == 0 {
		return nil, errors.New("relay credentials name no TURN server")
	}
	conf := c.base.CloneVT()
	if conf == nil {
		conf = &webrtc.WebRtcConfig{}
	}
	conf.IceServers = append(conf.IceServers, relays...)
	return conf, nil
}

// Close releases the controller.
func (c *Controller) Close() error { return nil }

// _ is a type assertion
var (
	_ controller.Controller = (*Controller)(nil)
	_ webrtc.ICEProvider    = (*Controller)(nil)
)
