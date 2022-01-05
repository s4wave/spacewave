package identity

import (
	"github.com/aperturerobotics/bifrost/hash"
	peer "github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/pkg/errors"
)

// NewEntity constructs a new entity object.
func NewEntity(entityID, entityUUID, domainID string) *Entity {
	return &Entity{
		EntityId:   entityID,
		EntityUuid: entityUUID,
		DomainId:   domainID,
		Epoch:      1,
	}
}

// NewEntityBlock constructs a new Entity block
func NewEntityBlock() block.Block {
	return &Entity{}
}

// UnmarshalEntity unmarshals a Entity from a cursor.
// If empty, returns nil, nil
func UnmarshalEntity(bcs *block.Cursor) (*Entity, error) {
	if bcs == nil {
		return nil, nil
	}
	blk, err := bcs.Unmarshal(NewEntityBlock)
	if err != nil {
		return nil, err
	}
	if blk == nil {
		return nil, nil
	}
	bv, ok := blk.(*Entity)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return bv, nil
}

// AppendKeypair adds a keypair to the entity.
//
// Signs the keypair + entity data using the private key.
// The private key must match the given keypair.
// The keypair must not already exist.
func (e *Entity) AppendKeypair(privKey crypto.PrivKey, kp *Keypair) error {
	// validate keypair
	if err := kp.Validate(); err != nil {
		return err
	}
	if err := kp.CheckMatchesEntity(e); err != nil {
		return err
	}
	// ensure that peer ids match
	expectedPeerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return err
	}
	expectedPeerIDPretty := expectedPeerID.Pretty()
	if kpPeerID := kp.GetPeerId(); expectedPeerIDPretty != kpPeerID {
		return errors.Errorf("private key %s does not match keypair %s", expectedPeerIDPretty, kpPeerID)
	}

	// sign the keypair data w/ the private key
	kpData, err := kp.MarshalBlock()
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
	for i, kpData := range e.GetKeypairs() {
		ekp := &Keypair{}
		var peerID peer.ID
		err := ekp.UnmarshalBlock(kpData)
		if err == nil {
			peerID, err = ekp.ParsePeerID()
		}
		if err == nil && len(peerID) == 0 {
			err = peer.ErrPeerIDEmpty
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
	e.Keypairs = append(e.Keypairs, kpData)
	e.KeypairSignatures = append(e.KeypairSignatures, sig)
	return nil
}

// UnmarshalVerifyKeypairs unmarshals and checks the keypair signatures.
func (e *Entity) UnmarshalVerifyKeypairs() ([]*Keypair, error) {
	keypairs := e.GetKeypairs()
	kpLen := len(keypairs)
	keypairSigs := e.GetKeypairSignatures()
	sigLen := len(keypairSigs)
	if kpLen != sigLen {
		return nil, errors.Errorf("keypairs count must match signatures count: %d != %d", kpLen, sigLen)
	}
	keypairVals := make([]*Keypair, len(keypairs))
	for i, kpData := range keypairs {
		kp := &Keypair{}
		if err := kp.UnmarshalBlock(kpData); err != nil {
			return nil, errors.Wrapf(err, "keypairs[%d]", i)
		}
		if err := kp.Validate(); err != nil {
			return nil, errors.Wrapf(err, "keypairs[%d]", i)
		}
		if err := kp.CheckMatchesEntity(e); err != nil {
			return nil, errors.Wrapf(err, "keypairs[%d]", i)
		}
		keypairVals[i] = kp
	}
	for i, kpSig := range keypairSigs {
		kp := keypairVals[i]
		pubKey, err := kpSig.ParsePubKey()
		if err != nil {
			return nil, errors.Wrapf(err, "keypair_signatures[%d]: pubkey:", i)
		}
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

// Validate validates the entity object and all keypair signatures.
// Auth method params and/or IDs are not validated.
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
	if _, err := e.UnmarshalVerifyKeypairs(); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Entity) MarshalBlock() ([]byte, error) {
	return proto.Marshal(e)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Entity) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, e)
}

// _ is a type assertion
var _ block.Block = ((*Entity)(nil))
