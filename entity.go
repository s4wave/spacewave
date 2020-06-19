package identity

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/pkg/errors"
)

// Validate validates the entity object (cursory validation).
// Auth method params or IDs are not validated.
func (e *Entity) Validate() error {
	if err := ValidateDomainID(e.GetDomainId()); err != nil {
		return err
	}
	if err := ValidateEntityID(e.GetEntityId()); err != nil {
		return err
	}
	if err := ValidateUUID(e.GetEntityUuid()); err != nil {
		return err
	}
	for _, kp := range e.GetKeypairs() {
		if len(kp.GetPeerId()) == 0 {
			return errors.New("keypair peer id cannot be empty")
		}
		if kp.GetAuthMethodId() == "" {
			if len(kp.GetAuthMethodParams()) != 0 {
				return errors.New("auth provider params cannot be set unless auth provider id is set")
			}
		}
		_, err := peer.IDB58Decode(kp.GetPeerId())
		if err != nil {
			return errors.Wrap(err, "decode peer id")
		}
	}
	return nil
}
