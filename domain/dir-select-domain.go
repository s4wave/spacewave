package identity_domain

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
)

// SelectIdentityDomain asks the user to select an identity domain.
type SelectIdentityDomain interface {
	// Directive indicates this is a directive.
	directive.Directive

	// SelectIdentityDomainPurpose is the purpose of the SelectIdentityDomain.
	// Current: "auth"
	SelectIdentityDomainPurpose() string
}

// SelectIdentityDomainValue is the result of the SelectIdentityDomain directive.
type SelectIdentityDomainValue = *DomainInfo

// ExSelectIdentityDomain executes the select entity domain directive.
func ExSelectIdentityDomain(ctx context.Context, b bus.Bus, purpose string) (SelectIdentityDomainValue, error) {
	av, _, dirRef, err := bus.ExecOneOff(ctx, b, NewSelectIdentityDomain(purpose), nil, nil)
	if err != nil {
		return nil, err
	}
	val := av.GetValue()
	dirRef.Release()
	valObj, valObjOk := val.(SelectIdentityDomainValue)
	if !valObjOk {
		return nil, block.ErrUnexpectedType
	}
	return valObj, nil
}

// selectIdentityDomain implements SelectIdentityDomain
type selectIdentityDomain struct {
	// purpose is the purpose id
	purpose string
}

// NewSelectIdentityDomain constructs a new SelectIdentityDomain directive.
func NewSelectIdentityDomain(purpose string) SelectIdentityDomain {
	return &selectIdentityDomain{purpose: purpose}
}

// SelectIdentityDomainPurpose is the purpose of the SelectIdentityDomain.
// Current: "auth"
func (s *selectIdentityDomain) SelectIdentityDomainPurpose() string {
	return s.purpose
}

// Validate checks the directive.
func (s *selectIdentityDomain) Validate() error { return nil }

// GetValueLookupLoggerOptions returns options relating to value handling.
func (s *selectIdentityDomain) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		MaxValueCount:   1,
		MaxValueHardCap: true,
	}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (s *selectIdentityDomain) IsEquivalent(other directive.Directive) bool {
	return false
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (s *selectIdentityDomain) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (s *selectIdentityDomain) GetName() string {
	return "SelectIdentityDomain"
}

// GetDebugVals returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (s *selectIdentityDomain) GetDebugVals() directive.DebugValues {
	return nil
}

// _ is a type assertion
var _ SelectIdentityDomain = ((*selectIdentityDomain)(nil))
