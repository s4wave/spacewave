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
	// Fail on stat errors other than a missing file, such as an unreadable
	// path component.
	_, statErr := os.Stat(privKeyPath)
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}

	// Load and parse the existing private-key file when present.
	if statErr == nil {
		dat, err := os.ReadFile(privKeyPath)
		if err != nil {
			return nil, err
		}
		return keypem.ParsePrivKeyPem(dat)
	}

	// Generate and persist a new key when the configured file is absent.
	le.Debug("generating priv key")
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	dat, err := keypem.MarshalPrivKeyPem(privKey)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(privKeyPath, dat, 0o600); err != nil {
		return nil, err
	}
	le.Debug("wrote private key")
	return privKey, nil
}
