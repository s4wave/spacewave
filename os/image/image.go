// Package os_image authorizes a Spacewave OS image for boot.
//
// The Space owner signs the image's root block, which identifies the boot
// set, profile and provenance beneath it. The bucket and transforms that locate
// the root may change without changing what was authorized.
package os_image

import (
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// ImageRootContext separates image authorizations from every other signature
// a Space owner's key makes.
const ImageRootContext = "spacewave/os 2026-11-19 image root authorization"

// SignImageRoot authorizes booting the image with this exact root.
func SignImageRoot(key crypto.PrivKey, root *block.BlockRef) (*peer.SignedMsg, error) {
	data, err := MarshalImageRoot(root)
	if err != nil {
		return nil, err
	}
	return peer.NewSignedMsg(ImageRootContext, key, hash.RecommendedHashType, data)
}

// MarshalImageRoot returns the bytes an image authorization signs in
// ImageRootContext, requiring a complete content-addressed root, so a signer
// that holds its key elsewhere signs exactly what SignImageRoot would.
func MarshalImageRoot(root *block.BlockRef) ([]byte, error) {
	if root.GetEmpty() {
		return nil, errors.New("image authorization: missing image root")
	}
	if err := root.Validate(false); err != nil {
		return nil, errors.Wrap(err, "image authorization: image root")
	}
	return root.MarshalVT()
}

// VerifyImageRoot returns the image root that a pinned signer authorized,
// with that signer. A valid signature from any other peer authorizes nothing.
func VerifyImageRoot(signed *peer.SignedMsg, pinned []peer.ID) (*block.BlockRef, peer.ID, error) {
	// Require the caller's pins before trusting any signature.
	if len(pinned) == 0 {
		return nil, "", errors.New("image authorization: no pinned signers")
	}

	// Authenticate the signature before trusting its signer or payload.
	_, signer, err := signed.ExtractAndVerify(ImageRootContext)
	if err != nil {
		return nil, "", errors.Wrap(err, "image authorization: invalid signature")
	}
	if !slices.Contains(pinned, signer) {
		return nil, "", errors.Errorf("image authorization: signer %s is not pinned", signer.String())
	}

	// Decode the authorized root.
	root := &block.BlockRef{}
	if err := root.UnmarshalVT(signed.GetData()); err != nil {
		return nil, "", errors.Wrap(err, "image authorization: image root")
	}
	if err := root.Validate(false); err != nil {
		return nil, "", errors.Wrap(err, "image authorization: image root")
	}
	return root, signer, nil
}
