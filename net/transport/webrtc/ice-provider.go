package webrtc

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/peer"
)

// ICEProvider supplies fresh ICE credentials for each new peer connection.
// Credentials remain local and must not be included in directive diagnostics.
type ICEProvider interface {
	// ICEConfig returns fresh local credentials for one connection attempt.
	ICEConfig(context.Context) (*WebRtcConfig, error)
}

// LookupICEProvider resolves the optional credential provider for one local peer.
// Value: ICEProvider. An idle lookup with no value uses the transport config.
type LookupICEProvider interface {
	directive.Directive
	// ICEPeerID identifies the local session whose credentials are requested.
	ICEPeerID() peer.ID
}

// lookupICEProvider identifies one local peer's credential source.
type lookupICEProvider struct {
	// peerID is the local session identity.
	peerID peer.ID
}

// NewLookupICEProvider constructs a lookup scoped to one local peer.
func NewLookupICEProvider(id peer.ID) LookupICEProvider {
	return &lookupICEProvider{peerID: id}
}

// ExLookupICEProvider retains the provider until the returned release is called.
func ExLookupICEProvider(ctx context.Context, b bus.Bus, id peer.ID) (ICEProvider, func(), error) {
	value, _, ref, err := bus.ExecOneOffTyped[ICEProvider](ctx, b, NewLookupICEProvider(id), bus.ReturnIfIdle(true), nil)
	if err != nil {
		return nil, nil, err
	}
	if value == nil {
		if ref != nil {
			ref.Release()
		}
		return nil, nil, nil
	}
	return value.GetValue(), ref.Release, nil
}

// ICEPeerID returns the local credential identity.
func (d *lookupICEProvider) ICEPeerID() peer.ID { return d.peerID }

// Validate requires a local peer identity.
func (d *lookupICEProvider) Validate() error {
	if d.peerID == "" {
		return peer.ErrEmptyPeerID
	}
	return nil
}

// GetValueOptions disposes credential lookups when their caller releases them.
func (d *lookupICEProvider) GetValueOptions() directive.ValueOptions { return directive.ValueOptions{} }

// IsEquivalent merges lookups for the same local peer.
func (d *lookupICEProvider) IsEquivalent(other directive.Directive) bool {
	o, ok := other.(LookupICEProvider)
	return ok && o.ICEPeerID() == d.peerID
}

// Superceeds preserves other credential lookups.
func (d *lookupICEProvider) Superceeds(directive.Directive) bool { return false }

// GetName identifies credential lookups without exposing credentials.
func (d *lookupICEProvider) GetName() string { return "LookupICEProvider" }

// GetDebugVals returns only the public peer identity.
func (d *lookupICEProvider) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{"peer": []string{d.peerID.String()}}
}

// _ is a type assertion
var _ LookupICEProvider = (*lookupICEProvider)(nil)
