package all

import (
	"github.com/aperturerobotics/auth"
	"github.com/aperturerobotics/auth/scrypt-user-pass"
)

// GetImplementations returns all known implementations.
func GetImplementations() []auth.Strategy {
	return []auth.Strategy{
		&scryptuserpass.ScryptUserPass{},
	}
}
