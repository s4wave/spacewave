package account_settings

import "github.com/s4wave/spacewave/core/sobject"

// BodyType is the SharedObjectMeta body_type for account settings SOs.
const BodyType = "account-settings"

// BindingPurpose is the logical account-settings binding purpose.
const BindingPurpose = "account-settings"

// NewSharedObjectMeta returns the SharedObjectMeta for an account settings SO.
func NewSharedObjectMeta() *sobject.SharedObjectMeta {
	return &sobject.SharedObjectMeta{
		BodyType:       BodyType,
		AccountPrivate: true,
	}
}
