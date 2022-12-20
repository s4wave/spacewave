package identity

import (
	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/pkg/errors"
)

// AppendKeypair adds a keypair to the set.
//
// Signs the keypair + entity data using the private key.
// The private key must match the given keypair.
// The keypair must not already exist.
// If Entity != nil, checks if the Entity matches the keypair.
func (e *EntityKeypairSet) AppendKeypair(privKey crypto.PrivKey, ekp *EntityKeypair, ent *Entity) error {
	// validate keypair
	if err := ekp.Validate(); err != nil {
		return err
	}
	if ent != nil {
		if err := ekp.CheckMatchesEntity(ent); err != nil {
			return err
		}
	}
	// ensure that peer ids match
	kp := ekp.GetKeypair()
	expectedPeerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return err
	}
	expectedPeerIDPretty := expectedPeerID.Pretty()
	if kpPeerID := kp.GetPeerId(); expectedPeerIDPretty != kpPeerID {
		return errors.Errorf("private key %s does not match keypair %s", expectedPeerIDPretty, kpPeerID)
	}

	// sign the keypair data w/ the private key
	kpData, err := ekp.MarshalBlock()
	if err != nil {
		return err
	}
	sig, err := peer.NewSignature(privKey, hash.HashType_HashType_SHA256, kpData, true)
	if err != nil {
		return err
	}
	// verify the signature matches (sanity check)
	pubKey := privKey.GetPublic()
	_, err = sig.VerifyWithPublic(pubKey, kpData)
	if err != nil {
		return err
	}
	// ensure no keypair exists with the peer id
	for i, kpData := range e.GetEntityKeypairs() {
		ekp := &EntityKeypair{}
		var peerID peer.ID
		err := ekp.UnmarshalBlock(kpData)
		if err == nil {
			peerID, err = ekp.GetKeypair().ParsePeerID()
		}
		if err == nil && len(peerID) == 0 {
			err = peer.ErrEmptyPeerID
		}
		if err != nil {
			return errors.Wrapf(err, "keypairs[%d]", i)
		}
		peerIDPretty := peerID.Pretty()
		if peerIDPretty == kp.GetPeerId() || peerID.MatchesPublicKey(pubKey) {
			return errors.Wrapf(err, "keypairs[%d] already contains peer %s", i, kp.GetPeerId())
		}
	}

	// append the signature + keypair
	e.EntityKeypairs = append(e.EntityKeypairs, kpData)
	e.EntityKeypairSignatures = append(e.EntityKeypairSignatures, sig)
	return nil
}

// UnmarshalVerifyKeypairs unmarshals and checks the keypair signatures.
//
// If ent != nil, checks that the keypairs match the entity.
func (e *EntityKeypairSet) UnmarshalVerifyKeypairs(ent *Entity) ([]*EntityKeypair, error) {
	keypairs := e.GetEntityKeypairs()
	kpLen := len(keypairs)
	keypairSigs := e.GetEntityKeypairSignatures()
	sigLen := len(keypairSigs)
	if kpLen != sigLen {
		return nil, errors.Errorf("keypairs count must match signatures count: %d != %d", kpLen, sigLen)
	}
	keypairVals := make([]*EntityKeypair, len(keypairs))
	for i, kpData := range keypairs {
		ekp := &EntityKeypair{}
		if err := ekp.UnmarshalBlock(kpData); err != nil {
			return nil, errors.Wrapf(err, "keypairs[%d]", i)
		}
		if err := ekp.Validate(); err != nil {
			return nil, errors.Wrapf(err, "keypairs[%d]", i)
		}
		if ent != nil {
			if err := ekp.CheckMatchesEntity(ent); err != nil {
				return nil, errors.Wrapf(err, "keypairs[%d]", i)
			}
		}
		keypairVals[i] = ekp
	}
	for i, kpSig := range keypairSigs {
		ekp := keypairVals[i]
		pubKey, err := kpSig.ParsePubKey()
		if err != nil {
			return nil, errors.Wrapf(err, "keypair_signatures[%d]: pubkey:", i)
		}
		kp := ekp.GetKeypair()
		peerID, err := kp.ParsePeerID()
		if err != nil {
			return nil, errors.Wrapf(err, "keypair_signatures[%d]: peer id:", i)
		}
		if !peerID.MatchesPublicKey(pubKey) {
			return nil, errors.Errorf("keypair_signatures[%d]: public key does not match peer id %s", i, peerID.Pretty())
		}
		ok, err := kpSig.VerifyWithPublic(pubKey, keypairs[i])
		if err == nil && !ok {
			err = errors.New("public key verify failed")
		}
		if err != nil {
			return nil, errors.Wrapf(err, "keypair_signatures[%d]: invalid sig:", i)
		}
	}
	return keypairVals, nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *EntityKeypairSet) MarshalBlock() ([]byte, error) {
	return e.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *EntityKeypairSet) UnmarshalBlock(data []byte) error {
	return e.UnmarshalVT(data)
}

// Validate validates the EntityKeypairSet.
//
// If ent != nil checks that the keypairs match the entity.
func (e *EntityKeypairSet) Validate(ent *Entity) error {
	_, err := e.UnmarshalVerifyKeypairs(ent)
	return err
}

// _ is a type assertion
var _ block.Block = ((*EntityKeypairSet)(nil))
