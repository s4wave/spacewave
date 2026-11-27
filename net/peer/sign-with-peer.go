package peer

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
)

// SignWithPeer is a directive asking the holder of a peer's private key to
// sign data in a signing context. The key stays with its holder. Only the
// SignedMsg crosses the bus.
//
// The asker chooses the context, so a holder answers only on buses whose
// askers it trusts to sign as the peer, and a bridge forwards the directive
// only to the holder it means to reach.
type SignWithPeer interface {
	// Directive indicates SignWithPeer is a directive.
	directive.Directive

	// SignWithPeerID returns the peer whose key signs.
	SignWithPeerID() ID
	// SignWithPeerContext returns the signing context, a hardcoded constant
	// as NewSignedMsg describes.
	SignWithPeerContext() string
	// SignWithPeerData returns the data to sign.
	SignWithPeerData() []byte
}

// SignWithPeerValue is the result of the SignWithPeer directive.
type SignWithPeerValue = *SignedMsg

// signWithPeer implements SignWithPeer.
type signWithPeer struct {
	peerID     ID
	encContext string
	data       []byte
}

// NewSignWithPeer constructs a SignWithPeer directive.
func NewSignWithPeer(peerID ID, encContext string, data []byte) SignWithPeer {
	return &signWithPeer{peerID: peerID, encContext: encContext, data: data}
}

// ExSignWithPeer signs data in encContext with peerID's key, through whichever
// holder of the key answers on b. It fails when no holder answers.
func ExSignWithPeer(ctx context.Context, b bus.Bus, peerID ID, encContext string, data []byte) (*SignedMsg, error) {
	// Wait until the bus settles, failing if no holder answered.
	signed, _, ref, err := bus.ExecWaitValue[SignWithPeerValue](
		ctx,
		b,
		NewSignWithPeer(peerID, encContext, data),
		bus.ReturnWhenIdle(),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, errors.Errorf("no holder of the key for peer %s answered", peerID.String())
	}
	ref.Release()
	return signed, nil
}

// SignWithPeerID returns the peer whose key signs.
func (d *signWithPeer) SignWithPeerID() ID { return d.peerID }

// SignWithPeerContext returns the signing context.
func (d *signWithPeer) SignWithPeerContext() string { return d.encContext }

// SignWithPeerData returns the data to sign.
func (d *signWithPeer) SignWithPeerData() []byte { return d.data }

// Validate requires a peer and a signing context.
func (d *signWithPeer) Validate() error {
	if err := d.peerID.Validate(); err != nil {
		return err
	}
	if d.encContext == "" {
		return errors.New("signing context is empty")
	}
	return nil
}

// GetValueOptions takes the first signature and disposes the directive with
// its last reference.
func (d *signWithPeer) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{MaxValueCount: 1, UnrefDisposeEmptyImmediate: true}
}

// IsEquivalent shares one signature among identical requests.
func (d *signWithPeer) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(SignWithPeer)
	return ok &&
		od.SignWithPeerID() == d.peerID &&
		od.SignWithPeerContext() == d.encContext &&
		bytes.Equal(od.SignWithPeerData(), d.data)
}

// Superceeds leaves other requests independent.
func (d *signWithPeer) Superceeds(other directive.Directive) bool { return false }

// GetName returns the directive's type name.
func (d *signWithPeer) GetName() string { return "SignWithPeer" }

// GetDebugVals identifies the signer and context, not the data.
func (d *signWithPeer) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{"peer-id": []string{d.peerID.String()}, "context": []string{d.encContext}}
}

// NewSignWithPeerResolver returns a resolver signing with key, or nil when
// key does not belong to the directive's peer.
func NewSignWithPeerResolver(d SignWithPeer, key crypto.PrivKey) directive.Resolver {
	id, err := IDFromPrivateKey(key)
	if err != nil || id != d.SignWithPeerID() {
		return nil
	}
	return directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		signed, err := NewSignedMsg(d.SignWithPeerContext(), key, hash.RecommendedHashType, d.SignWithPeerData())
		if err != nil {
			return err
		}
		_, _ = handler.AddValue(signed)
		return nil
	})
}

// _ is a type constraint.
var (
	_ SignWithPeer         = (*signWithPeer)(nil)
	_ directive.Debuggable = (*signWithPeer)(nil)
)
