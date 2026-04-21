package peer_ssh

import (
	"github.com/s4wave/spacewave/net/crypto"
	"golang.org/x/crypto/ssh"
)

// NewPublicKey converts a public key into a ssh public key.
// Returns ErrKeyUnsupported if the key type is unsupported.
func NewPublicKey(pubKey crypto.PubKey) (ssh.PublicKey, error) {
	stdKey, err := crypto.PubKeyToStdKey(pubKey)
	if err != nil {
		return nil, err
	}
	return ssh.NewPublicKey(stdKey)
}
