package link_solicit

import (
	"context"
	"strconv"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
)

// holdOpenDur is the default hold open duration for SolicitProtocol.
var holdOpenDur = time.Second * 10

// SolicitProtocol is a directive to solicit a protocol with peers.
//
// When both sides emit SolicitProtocol for the same protocol (matching
// hash), a stream is established and both directives resolve with a
// SolicitMountedStream value.
//
// Value type: SolicitMountedStream
type SolicitProtocol interface {
	// Directive indicates SolicitProtocol is a directive.
	directive.Directive

	// SolicitProtocolID returns the protocol ID to solicit.
	SolicitProtocolID() protocol.ID
	// SolicitProtocolContext returns the opaque context for the solicitation.
	// Different contexts produce different hashes (no match).
	SolicitProtocolContext() []byte
	// SolicitProtocolPeerID returns the optional peer ID constraint.
	// If empty, solicits on all active links.
	SolicitProtocolPeerID() peer.ID
	// SolicitProtocolTransportID returns the optional transport constraint.
	// If 0, solicits on all transports.
	SolicitProtocolTransportID() uint64
}

// SolicitProtocolValue is the type emitted when resolving SolicitProtocol.
type SolicitProtocolValue = SolicitMountedStream

// solicitProtocol implements SolicitProtocol.
type solicitProtocol struct {
	protocolID  protocol.ID
	context     []byte
	peerID      peer.ID
	transportID uint64
}

// NewSolicitProtocol constructs a new SolicitProtocol directive.
func NewSolicitProtocol(
	protocolID protocol.ID,
	ctx []byte,
	peerID peer.ID,
	transportID uint64,
) SolicitProtocol {
	return &solicitProtocol{
		protocolID:  protocolID,
		context:     ctx,
		peerID:      peerID,
		transportID: transportID,
	}
}

// SolicitProtocolID returns the protocol ID to solicit.
func (d *solicitProtocol) SolicitProtocolID() protocol.ID {
	return d.protocolID
}

// SolicitProtocolContext returns the opaque context for the solicitation.
func (d *solicitProtocol) SolicitProtocolContext() []byte {
	return d.context
}

// SolicitProtocolPeerID returns the optional peer ID constraint.
func (d *solicitProtocol) SolicitProtocolPeerID() peer.ID {
	return d.peerID
}

// SolicitProtocolTransportID returns the optional transport constraint.
func (d *solicitProtocol) SolicitProtocolTransportID() uint64 {
	return d.transportID
}

// Validate validates the directive.
func (d *solicitProtocol) Validate() error {
	if len(d.protocolID) == 0 {
		return errors.New("protocol id required")
	}
	if err := d.protocolID.Validate(); err != nil {
		return errors.Wrap(err, "protocol_id")
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *solicitProtocol) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur: holdOpenDur,
	}
}

// IsEquivalent checks if the other directive is equivalent.
func (d *solicitProtocol) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(SolicitProtocol)
	if !ok {
		return false
	}

	if d.protocolID != od.SolicitProtocolID() {
		return false
	}
	if d.peerID != od.SolicitProtocolPeerID() {
		return false
	}
	if string(d.context) != string(od.SolicitProtocolContext()) {
		return false
	}
	return true
}

// Superceeds checks if the directive overrides another.
func (d *solicitProtocol) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
func (d *solicitProtocol) GetName() string {
	return "SolicitProtocol"
}

// GetDebugVals returns the directive arguments as k/v pairs.
func (d *solicitProtocol) GetDebugVals() directive.DebugValues {
	vals := directive.NewDebugValues()
	vals["protocol-id"] = []string{string(d.protocolID)}
	if len(d.peerID) != 0 {
		vals["peer-id"] = []string{d.peerID.String()}
	}
	if d.transportID != 0 {
		vals["transport-id"] = []string{strconv.FormatUint(d.transportID, 10)}
	}
	return vals
}

// ExSolicitProtocol executes the SolicitProtocol directive, waiting for a
// single matched stream. Returns the SolicitMountedStream value, the directive
// instance, a reference to release, and any error.
func ExSolicitProtocol(
	ctx context.Context,
	b bus.Bus,
	protocolID protocol.ID,
	bctx []byte,
	peerID peer.ID,
	transportID uint64,
) (SolicitMountedStream, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[SolicitMountedStream](
		ctx,
		b,
		NewSolicitProtocol(protocolID, bctx, peerID, transportID),
		nil,
		nil,
		nil,
	)
}

// _ is a type assertion
var _ SolicitProtocol = ((*solicitProtocol)(nil))
