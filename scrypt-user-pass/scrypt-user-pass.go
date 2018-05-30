package scryptuserpass

import (
	"github.com/aperturerobotics/auth"
)

// ScryptUserPass is the scrypt-user-pass auth implementation.
type ScryptUserPass struct{}

// GetAuthType returns the encryption type this implementation satisfies.
func (a *ScryptUserPass) GetAuthType() auth.AuthType {
	return auth.AuthType_AuthType_SCRYPT_USER_PASS
}

func init() {
	auth.MustRegisterStrategy(&ScryptUserPass{})
}
