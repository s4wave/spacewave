package auth_challenge_client

import (
	"errors"

	auth_challenge "github.com/aperturerobotics/auth/challenge"
	"github.com/aperturerobotics/identity"
)

// LookupEntityValue is the result of a lookup entity request.
type LookupEntityValue struct {
	proto *auth_challenge.EntityLookupFinish
}

// NewLookupEntityValue constructs a new LookupEntityValue
func NewLookupEntityValue(val *auth_challenge.EntityLookupFinish) *LookupEntityValue {
	return &LookupEntityValue{
		proto: val,
	}
}

// GetError returns any overall error with the process.
func (v *LookupEntityValue) GetError() error {
	if errStr := v.proto.GetLookupError(); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// IsNotFound indicates if the result was not-found.
// If this is set and err != nil, err must be a not found error.
func (v *LookupEntityValue) IsNotFound() bool {
	return v.proto.GetLookupIsNotFound()
}

// GetEntity returns the entity record that was found.
func (v *LookupEntityValue) GetEntity() *identity.Entity {
	return v.proto.GetLookupEntity()
}

// _ is a type assertion
var _ identity.IdentityLookupEntityValue = ((*LookupEntityValue)(nil))
