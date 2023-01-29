package identity

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
)

// SelectIdentityEntity asks the domain controller to select an entity.
type SelectIdentityEntity interface {
	// Directive indicates this is a directive.
	directive.Directive

	// SelectIdentityEntityPurpose is the purpose of the SelectIdentityEntity.
	// Current: "auth"
	SelectIdentityEntityPurpose() string
	// SelectIdentityEntityDomainID is the domain id to select an entity.
	SelectIdentityEntityDomainID() string
	// SelectIdentityEntityPrevError is the error for the previous attempt.
	// Usually nil.
	SelectIdentityEntityPrevError() error
}

// SelectIdentityEntityValue is the result of the SelectIdentityEntity directive.
// Note: the pointer might be nil if no entity was selected.
type SelectIdentityEntityValue = *Entity

// ExSelectIdentityEntity executes the select entity directive.
func ExSelectIdentityEntity(
	ctx context.Context,
	b bus.Bus,
	purpose string,
	domainID string,
	prevErr error,
) (SelectIdentityEntityValue, error) {
	av, _, dirRef, err := bus.ExecOneOff(ctx, b, NewSelectIdentityEntity(purpose, domainID, prevErr), false, nil)
	if err != nil {
		return nil, err
	}
	val := av.GetValue()
	dirRef.Release()
	valObj, valObjOk := val.(SelectIdentityEntityValue)
	if !valObjOk {
		return nil, block.ErrUnexpectedType
	}
	return valObj, nil
}

// selectIdentityEntity implements SelectIdentityEntity
type selectIdentityEntity struct {
	purpose  string
	domainID string
	prevErr  error
}

// NewSelectIdentityEntity constructs a new SelectIdentityEntity directive.
func NewSelectIdentityEntity(purpose, domainID string, prevErr error) SelectIdentityEntity {
	return &selectIdentityEntity{
		purpose:  purpose,
		domainID: domainID,
		prevErr:  prevErr,
	}
}

// SelectIdentityEntityPurpose is the purpose of the SelectIdentityEntity.
// Current: "auth"
func (s *selectIdentityEntity) SelectIdentityEntityPurpose() string {
	return s.purpose
}

// SelectIdentityEntityDomainID is the domain id to select.
func (s *selectIdentityEntity) SelectIdentityEntityDomainID() string {
	return s.domainID
}

// SelectIdentityEntityPrevError is the error for the previous attempt.
// Usually nil.
func (s *selectIdentityEntity) SelectIdentityEntityPrevError() error {
	return s.prevErr
}

// Validate checks the directive.
func (s *selectIdentityEntity) Validate() error { return nil }

// GetValueLookupLoggerOptions returns options relating to value handling.
func (s *selectIdentityEntity) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		MaxValueCount:   1,
		MaxValueHardCap: true,
	}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (s *selectIdentityEntity) IsEquivalent(other directive.Directive) bool {
	return false
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (s *selectIdentityEntity) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (s *selectIdentityEntity) GetName() string {
	return "SelectIdentityEntity"
}

// GetDebugVals returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (s *selectIdentityEntity) GetDebugVals() directive.DebugValues {
	return nil
}

// _ is a type assertion
var _ SelectIdentityEntity = ((*selectIdentityEntity)(nil))
