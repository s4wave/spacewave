// Package os_image defines the signed manifest that names a Spacewave OS
// image.
package os_image

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// manifestSignatureContext separates manifest signatures from every other
// signature a Space owner's key makes.
const manifestSignatureContext = "spacewave 2026-11-19 os root manifest v1"

// Sign signs the manifest with the producer's key, replacing any previous
// signature.
func (m *OSRootManifest) Sign(key crypto.PrivKey) error {
	// A manifest names an image, so an empty one is never signed.
	if m.GetImage().GetRootRef().GetEmpty() {
		return errors.New("manifest names no image root")
	}

	// Sign everything but the signature itself.
	data, err := m.signedBody()
	if err != nil {
		return err
	}
	sig, err := peer.NewSignature(manifestSignatureContext, key, hash.RecommendedHashType, data, true)
	if err != nil {
		return err
	}
	m.Signature = sig
	return nil
}

// Verify checks the manifest's signature and returns its signer. The caller
// decides whether to trust that signer, such as against a pinned set.
func (m *OSRootManifest) Verify() (peer.ID, error) {
	// Require a signature and an image to check it over.
	if m.GetSignature() == nil {
		return "", errors.New("manifest is unsigned")
	}
	if m.GetImage().GetRootRef().GetEmpty() {
		return "", errors.New("manifest names no image root")
	}

	// Check the signature over everything but itself.
	data, err := m.signedBody()
	if err != nil {
		return "", err
	}
	signer, err := m.GetSignature().Verify(manifestSignatureContext, data)
	if err != nil {
		return "", errors.Wrap(err, "manifest signature")
	}
	return signer, nil
}

// signedBody encodes the manifest with its signature cleared.
func (m *OSRootManifest) signedBody() ([]byte, error) {
	unsigned := m.CloneVT()
	unsigned.Signature = nil
	return unsigned.MarshalVT()
}
