package keyfile

import (
	"crypto/rand"
	"os"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/sirupsen/logrus"
)

// OpenOrWritePrivKey opens or generates a private key at a path.
// Uses PEM format and ed25519 keys.
// May return a private key + an error.
func OpenOrWritePrivKey(le *logrus.Entry, privKeyPath string) (crypto.PrivKey, error) {
	var privKey crypto.PrivKey
	var err error
	if _, err := os.Stat(privKeyPath); err != nil {
		if os.IsNotExist(err) {
			if le != nil {
				le.Debug("generating priv key")
			}
			privKey, _, err = crypto.GenerateEd25519Key(rand.Reader)
			if err != nil {
				return privKey, err
			}
			dat, err := keypem.MarshalPrivKeyPem(privKey)
			if err != nil {
				return privKey, err
			}
			if err := os.WriteFile(privKeyPath, dat, 0o600); err != nil {
				return privKey, err
			}
			if le != nil {
				le.Debug("wrote private key")
			}
		}
	} else {
		dat, err := os.ReadFile(privKeyPath)
		if err != nil {
			return privKey, err
		}
		privKey, err = keypem.ParsePrivKeyPem(dat)
		if err != nil {
			return privKey, err
		}
	}
	return privKey, err
}
