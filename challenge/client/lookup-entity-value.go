package auth_challenge_client

import (
	"errors"

	auth_challenge "github.com/aperturerobotics/auth/challenge"
	"github.com/aperturerobotics/identity"
)

// lookupEntityValue is the result of a lookup entity request
type lookupEntityValue struct {
	proto *auth_challenge.EntityLookupFinish
}

// newLookupEntityValue constructs a new lookupEntityValue
func newLookupEntityValue(val *auth_challenge.EntityLookupFinish) *lookupEntityValue {
	return &lookupEntityValue{
		proto: val,
	}
}

// GetError returns any overall error with the process.
func (v *lookupEntityValue) GetError() error {
	if errStr := v.proto.GetLookupError(); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// IsNotFound indicates if the result was not-found.
// If this is set and err != nil, err must be a not found error.
func (v *lookupEntityValue) IsNotFound() bool {
	return v.proto.GetLookupIsNotFound()
}

// GetEntity returns the entity record that was found.
func (v *lookupEntityValue) GetEntity() *identity.Entity {
	return v.proto.GetLookupEntity()
}

// _ is a type assertion
var _ identity.IdentityLookupEntityValue = ((*lookupEntityValue)(nil))
