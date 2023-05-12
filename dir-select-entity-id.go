package identity

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
)

// SelectEntityId asks the user to enter a entity id in a domain.
type SelectEntityId interface {
	// Directive indicates this is a directive.
	directive.Directive

	// SelectEntityIdPurpose is the purpose of the SelectEntityId.
	// Current: "auth"
	SelectEntityIdPurpose() string
	// SelectEntityIdDomainID is the domain id to select an entity.
	SelectEntityIdDomainID() string
	// SelectEntityIdPrevError is the error for the previous attempt.
	// Usually empty.
	SelectEntityIdPrevError() error
}

// SelectEntityIdValue is the result of the SelectEntityId directive.
// Note: the pointer might be nil if no entity was selected.
type SelectEntityIdValue = string

// ExSelectEntityId executes the select entity id directive.
func ExSelectEntityId(ctx context.Context, b bus.Bus, purpose, domainID string, prevErr error) (SelectEntityIdValue, error) {
	av, _, dirRef, err := bus.ExecOneOff(ctx, b, NewSelectEntityId(purpose, domainID, prevErr), nil, nil)
	if err != nil {
		return "", err
	}
	val := av.GetValue()
	dirRef.Release()
	valObj, valObjOk := val.(SelectEntityIdValue)
	if !valObjOk {
		return "", block.ErrUnexpectedType
	}
	return valObj, nil
}

// selectEntityId implements SelectEntityId
type selectEntityId struct {
	purpose  string
	domainID string
	prevErr  error
}

// NewSelectEntityId constructs a new SelectEntityId directive.
func NewSelectEntityId(purpose, domainID string, prevErr error) SelectEntityId {
	return &selectEntityId{purpose: purpose, domainID: domainID, prevErr: prevErr}
}

// SelectEntityIdPurpose is the purpose of the SelectEntityId.
// Current: "auth"
func (s *selectEntityId) SelectEntityIdPurpose() string {
	return s.purpose
}

// SelectEntityIdDomainID is the domain id to select.
func (s *selectEntityId) SelectEntityIdDomainID() string {
	return s.domainID
}

// SelectEntityIdPrevError is the error for the previous attempt.
// Usually empty.
func (s *selectEntityId) SelectEntityIdPrevError() error {
	return s.prevErr
}

// Validate checks the directive.
func (s *selectEntityId) Validate() error { return nil }

// GetValueLookupLoggerOptions returns options relating to value handling.
func (s *selectEntityId) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		MaxValueCount:   1,
		MaxValueHardCap: true,
	}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (s *selectEntityId) IsEquivalent(other directive.Directive) bool {
	return false
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (s *selectEntityId) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (s *selectEntityId) GetName() string {
	return "SelectEntityId"
}

// GetDebugVals returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (s *selectEntityId) GetDebugVals() directive.DebugValues {
	return nil
}

// _ is a type assertion
var _ SelectEntityId = ((*selectEntityId)(nil))
