package identity

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

// ValidateDomainID checks if a domain ID is valid.
func ValidateDomainID(id string) error {
	if id == "" {
		return errors.New("domain id cannot be empty")
	}
	// TODO additional verification
	return nil
}

// ValidateEntityID checks if a entity ID (username) is valid.
func ValidateEntityID(id string) error {
	if id == "" {
		return errors.New("entity id cannot be empty")
	}
	// TODO additional verification
	return nil
}

// ValidateUUID checks if a uuid is valid.
func ValidateUUID(id string) error {
	// TODO additional verification
	_, err := uuid.FromString(id)
	return err
}

// ValidateDomainUUID checks if the domain-specific UUID is valid.
func ValidateDomainUUID(id string) error {
	return ValidateUUID(id)
}

// ValidatePeerID checks if a peer ID is valid.
func ValidatePeerID(id string) error {
	pid, err := confparse.ParsePeerID(id)
	if err == nil && len(pid) == 0 {
		err = errors.New("peer id cannot be empty")
	}
	return err
}

// ValidatePubKey checks if a public key is set and valid.
//
// if the peer id is given, checks if it matches
func ValidatePubKey(id string, peerID peer.ID) error {
	pkey, err := confparse.ParsePublicKey(id)
	if err == nil && pkey == nil {
		err = errors.New("pub_key cannot be empty")
	}
	if err != nil || len(peerID) == 0 {
		return err
	}

	if !peerID.MatchesPublicKey(pkey) {
		pkeyID, err := peer.IDFromPublicKey(pkey)
		if err != nil {
			return err
		}

		return errors.Errorf(
			"pub_key id %s does not match peer_id %s",
			pkeyID.Pretty(),
			peerID.Pretty(),
		)
	}

	return nil
}
