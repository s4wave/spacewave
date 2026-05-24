package entitykeylogin

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

// ParsePrivateKey parses entity-key login PEM.
func ParsePrivateKey(pemData []byte) (crypto.PrivKey, error) {
	if len(pemData) == 0 {
		return nil, errors.New("pem_private_key is required")
	}

	privKey, err := keypem.ParsePrivKeyPem(pemData)
	if err != nil {
		return nil, errors.Wrap(err, "parse PEM private key")
	}
	if privKey == nil {
		return nil, errors.New("pem_private_key must contain a PEM private key")
	}
	return privKey, nil
}
