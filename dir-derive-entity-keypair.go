package identity

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
)

// DeriveEntityKeypair asks any running controllers to derive a private key.
// Controllers should inspect the auth_method_id and auth_method_params.
// If no controllers derive the keypair, will return not found.
type DeriveEntityKeypair interface {
	// Directive indicates this is a directive.
	directive.Directive

	// DeriveEntityKeypairList is the list of keypairs to derive for.
	// Any of the keypairs can be resolved.
	// The entity id and domain id fields may be empty.
	DeriveEntityKeypairList() []*EntityKeypair
}

// DeriveEntityKeypairValue is a result of the DeriveEntityKeypair directive.
// The peer will be matched to the Keypair by peer ID.
type DeriveEntityKeypairValue = peer.Peer

// ExDeriveEntityKeypair executes the derive entity keypair directive.
//
// unrefDisposeDur is the duration to keep the keypair in memory.
// If unrefDisposeDur is negative, sets to the default value of 30 seconds.
func ExDeriveEntityKeypair(
	ctx context.Context,
	b bus.Bus,
	kps []*EntityKeypair,
	unrefDisposeDur time.Duration,
) ([]DeriveEntityKeypairValue, directive.Reference, error) {
	vals, dirRef, err := bus.ExecCollectValues(ctx, b, NewDeriveEntityKeypair(kps, unrefDisposeDur), nil)
	if err != nil {
		return nil, nil, err
	}
	res := make([]DeriveEntityKeypairValue, 0, len(vals))
	for _, v := range vals {
		dv, dvOk := v.(DeriveEntityKeypairValue)
		if dvOk {
			res = append(res, dv)
		}
	}
	return res, dirRef, nil
}

// ExDeriveKeypair executes the derive entity keypair directive w/o entity info.
//
// unrefDisposeDur is the duration to keep the keypair in memory.
// If unrefDisposeDur is negative, sets to the default value of 30 seconds.
func ExDeriveKeypair(
	ctx context.Context,
	b bus.Bus,
	kps []*Keypair,
	unrefDisposeDur time.Duration,
) ([]DeriveEntityKeypairValue, directive.Reference, error) {
	ekps := make([]*EntityKeypair, len(kps))
	for i, k := range kps {
		ekps[i] = &EntityKeypair{Keypair: k}
	}
	return ExDeriveEntityKeypair(ctx, b, ekps, unrefDisposeDur)
}

// deriveKeypair implements DeriveEntityKeypair
type deriveKeypair struct {
	// kps are the keypairs
	kps []*EntityKeypair
	// unrefDisposeDur is the duration to keep the keypair in memory.
	// If unrefDisposeDur is negative, sets to the default value of 30 seconds.
	unrefDisposeDur time.Duration
}

// NewDeriveEntityKeypair constructs a new DeriveEntityKeypair directive.
//
// unrefDisposeDur is the duration to keep the keypair in memory.
// If unrefDisposeDur is negative, sets to the default value of 30 seconds.
func NewDeriveEntityKeypair(kps []*EntityKeypair, unrefDisposeDur time.Duration) DeriveEntityKeypair {
	if unrefDisposeDur < 0 {
		unrefDisposeDur = time.Second * 10
	}
	return &deriveKeypair{
		kps:             kps,
		unrefDisposeDur: unrefDisposeDur,
	}
}

// DeriveEntityKeypairList is the list of keypairs to match to peers.
func (s *deriveKeypair) DeriveEntityKeypairList() []*EntityKeypair {
	return s.kps
}

// Validate checks the directive.
func (s *deriveKeypair) Validate() error {
	if len(s.kps) == 0 {
		return errors.New("at least one keypair is required")
	}
	for kpi, ekp := range s.kps {
		// we allow domain id + entity id to be empty here
		var err error
		if ekp.GetDomainId() == "" && ekp.GetEntityId() == "" {
			err = ekp.GetKeypair().Validate()
		} else {
			err = ekp.Validate()
		}
		if err != nil {
			return errors.Wrapf(err, "invalid keypair: keypairs[%d]", kpi)
		}
	}
	return nil
}

// GetValueLookupLoggerOptions returns options relating to value handling.
func (s *deriveKeypair) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		MaxValueCount:   1,
		MaxValueHardCap: true,

		UnrefDisposeDur:            s.unrefDisposeDur,
		UnrefDisposeEmptyImmediate: true,
	}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (s *deriveKeypair) IsEquivalent(other directive.Directive) bool {
	return false
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (s *deriveKeypair) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (s *deriveKeypair) GetName() string {
	return "DeriveEntityKeypair"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (s *deriveKeypair) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	var kpPeerIDs []string
	for _, kp := range s.kps {
		kpPeerIDs = append(kpPeerIDs, kp.GetKeypair().GetPeerId())
	}
	vals["keypairs"] = kpPeerIDs
	return vals
}

// _ is a type assertion
var _ DeriveEntityKeypair = ((*deriveKeypair)(nil))
