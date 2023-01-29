package identity

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
)

// IdentityLookupEntity is a directive to search for a entity record.
// At least one of the search fields should be set.
// TODO: For now the domain ID must be set.
//
// The entity record contains the list of keypairs which may contain information
// on how to derive the key, given a secret or local hardware private key. Note
// that it would not be possible to derive the private key without the secret
// for each auth method, for username this would be the password (scrypt key
// generation with proof of work).
type IdentityLookupEntity interface {
	// Directive indicates IdentityLookupEntity is a directive.
	directive.Directive

	// IdentityLookupEntityDomainID is the domain identifier.
	// Cannot be empty.
	IdentityLookupEntityDomainID() string

	// At least one of the below must be set.

	// IdentityLookupEntityID is the domain-unique identifier (username).
	IdentityLookupEntityID() string
}

// IdentityLookupEntityValue is the result of the IdentityLookupEntity directive.
type IdentityLookupEntityValue interface {
	// GetError returns any overall error with the process.
	GetError() error
	// IsNotFound indicates if the result was not-found.
	// If this is set and err != nil, err must be a not found error.
	IsNotFound() bool
	// GetEntity returns the entity record that was found.
	GetEntity() *Entity
}

// lookupEntityValue implements IdentityLookupEntityValue
type lookupEntityValue struct {
	err      error
	notFound bool
	ent      *Entity
}

// NewIdentityLookupEntityValue constructs a new lookupEntity value.
func NewIdentityLookupEntityValue(
	err error,
	notFound bool,
	ent *Entity,
) IdentityLookupEntityValue {
	return &lookupEntityValue{
		err:      err,
		notFound: notFound,
		ent:      ent,
	}
}

// GetError returns any overall error with the process.
func (v *lookupEntityValue) GetError() error {
	return v.err
}

// IsNotFound indicates if the result was not-found.
// If this is set and err != nil, err must be a not found error.
func (v *lookupEntityValue) IsNotFound() bool {
	return v.notFound
}

// GetEntity returns the entity record that was found.
func (v *lookupEntityValue) GetEntity() *Entity {
	return v.ent
}

// _ is a type assertion
var _ IdentityLookupEntityValue = ((*lookupEntityValue)(nil))

// lookupEntity implements IdentityLookupEntity with a global id constraint.
type lookupEntity struct {
	entityID string
	domainID string
}

// NewIdentityLookupEntity constructs a new lookupEntity directive.
func NewIdentityLookupEntity(
	domainID string,
	entityID string,
) IdentityLookupEntity {
	return &lookupEntity{
		domainID: domainID,
		entityID: entityID,
	}
}

// ExIdentityLookupEntity executes the lookup entity directive.
func ExIdentityLookupEntity(ctx context.Context, b bus.Bus, domainID, entityID string) (IdentityLookupEntityValue, error) {
	av, _, dirRef, err := bus.ExecOneOff(ctx, b, NewIdentityLookupEntity(domainID, entityID), false, nil)
	if err != nil {
		return nil, err
	}
	val := av.GetValue()
	dirRef.Release()
	valObj, valObjOk := val.(IdentityLookupEntityValue)
	if !valObjOk {
		return nil, block.ErrUnexpectedType
	}
	return valObj, nil
}

// IdentityLookupEntityDomainID is the domain ID.
// Cannot be empty.
func (d *lookupEntity) IdentityLookupEntityDomainID() string {
	return d.domainID
}

// IdentityLookupEntityID is the entity id.
func (d *lookupEntity) IdentityLookupEntityID() string {
	return d.entityID
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupEntity) Validate() error {
	if d.domainID == "" {
		return errors.New("domain id must be set")
	}
	if d.entityID == "" {
		return errors.New("entity id must be set")
	}

	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupEntity) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		MaxValueCount:   1,
		MaxValueHardCap: true,
	}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupEntity) IsEquivalent(other directive.Directive) bool {
	ot, ok := other.(*lookupEntity)
	if !ok {
		return false
	}

	return ot.domainID == d.domainID && ot.entityID == d.entityID
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupEntity) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupEntity) GetName() string {
	return "IdentityLookupEntity"
}

// GetDebugVals returns the directive arguments as key/value pairs.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupEntity) GetDebugVals() directive.DebugValues {
	vals := directive.NewDebugValues()
	vals["domain-id"] = []string{d.IdentityLookupEntityDomainID()}
	vals["entity-id"] = []string{d.IdentityLookupEntityID()}
	return vals
}

// _ is a type constraint
var _ IdentityLookupEntity = ((*lookupEntity)(nil))
