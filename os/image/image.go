// Package os_image authorizes a Spacewave OS image for boot.
//
// An image is a UnixFS tree stored under its Space's block transform. The
// Space owner signs a bucket.ObjectRef naming the tree's root block and that
// transform: the root hashes the encrypted bytes, and the transform reads
// them, so the signature fixes both the content and the key that opens it. A
// Device copies the encrypted blocks unchanged into its image store and reads
// them through the signed transform, so the authorization it boots from also
// carries the key. Never print or log a signed image reference whole; print
// its root block alone.
package os_image

import (
	"crypto/rand"
	"slices"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// ImageContext separates image authorizations from every other signature a
// Space owner's key makes.
const ImageContext = "spacewave/os 2026-11-24 image authorization"

// SignImage authorizes booting the image ref names: its exact root, read
// through its transform.
func SignImage(key crypto.PrivKey, ref *bucket.ObjectRef) (*peer.SignedMsg, error) {
	data, err := MarshalImage(ref)
	if err != nil {
		return nil, err
	}
	return peer.NewSignedMsg(ImageContext, key, hash.RecommendedHashType, data)
}

// MarshalImage returns the bytes an image authorization signs in ImageContext,
// after checking that ref names an image, so a signer that holds its key
// elsewhere signs exactly what SignImage would.
func MarshalImage(ref *bucket.ObjectRef) ([]byte, error) {
	if err := ValidateImage(ref); err != nil {
		return nil, err
	}
	return ref.MarshalVT()
}

// ValidateImage checks that ref names an image: a complete root block and an
// inline transform, with no bucket change and no transform reference, since
// the image store holds one bucket and the boot reads no other block to find
// the key.
func ValidateImage(ref *bucket.ObjectRef) error {
	// Require a complete root.
	root := ref.GetRootRef()
	if root.GetEmpty() {
		return errors.New("image: missing root")
	}
	if err := root.Validate(false); err != nil {
		return errors.Wrap(err, "image: root")
	}

	// Require the transform inline, and nothing else to look up.
	if ref.GetBucketId() != "" || !ref.GetTransformConfRef().GetEmpty() {
		return errors.New("image: names a bucket or a transform reference")
	}
	if ref.GetTransformConf().GetEmpty() {
		return errors.New("image: missing transform")
	}
	return errors.Wrap(ref.GetTransformConf().Validate(), "image: transform")
}

// VerifyImage returns the image a pinned signer authorized, with that signer.
// A valid signature from any other peer authorizes nothing.
func VerifyImage(signed *peer.SignedMsg, pinned []peer.ID) (*bucket.ObjectRef, peer.ID, error) {
	// Require the caller's pins before trusting any signature.
	if len(pinned) == 0 {
		return nil, "", errors.New("image authorization: no pinned signers")
	}

	// Authenticate the signature before trusting its signer or payload.
	_, signer, err := signed.ExtractAndVerify(ImageContext)
	if err != nil {
		return nil, "", errors.Wrap(err, "image authorization: invalid signature")
	}
	if !slices.Contains(pinned, signer) {
		return nil, "", errors.Errorf("image authorization: signer %s is not pinned", signer.String())
	}

	// Decode the authorized image.
	ref := &bucket.ObjectRef{}
	if err := ref.UnmarshalVT(signed.GetData()); err != nil {
		return nil, "", errors.Wrap(err, "image authorization")
	}
	if err := ValidateImage(ref); err != nil {
		return nil, "", errors.Wrap(err, "image authorization")
	}
	return ref, signer, nil
}

// NewTransform returns a block transform with a fresh random key, for an
// image built outside any Space, such as a rehearsal's or a test's. An image
// built in a Space uses the Space World's transform instead.
func NewTransform() (*block_transform.Config, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	defer scrub.Scrub(key)
	return block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{BlockEnc: blockenc.DefaultBlockEnc, Key: key},
	})
}
