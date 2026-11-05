package space_world_replica

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// proofSignContext is the signature context of a replica proof.
const proofSignContext = "sobject replica proof"

// SignReplicaProof signs proof with the key of the Device it names.
func SignReplicaProof(key crypto.PrivKey, proof *SOReplicaProof) (*SignedSOReplicaProof, error) {
	// Refuse a proof that names another Device.
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if proof.GetPeerId() != id.String() {
		return nil, errors.New("replica proof names another peer")
	}

	// Sign the encoded proof.
	data, err := proof.MarshalVT()
	if err != nil {
		return nil, err
	}
	sig, err := peer.NewSignature(proofSignContext, key, hash.RecommendedHashType, data, true)
	if err != nil {
		return nil, err
	}
	return &SignedSOReplicaProof{Proof: data, Signature: sig}, nil
}

// Verify returns the proof when it is for sharedObjectID and the stream peer,
// the proof's peer and the signer all name desiredOwner.
func (s *SignedSOReplicaProof) Verify(sharedObjectID, desiredOwner, streamPeer string) (*SOReplicaProof, error) {
	// Bound the work before parsing.
	if len(s.GetProof()) == 0 || s.SizeVT() > sobject.MaxSyncStateSize {
		return nil, errors.New("replica proof is empty or too large")
	}
	if streamPeer != desiredOwner {
		return nil, errors.New("replica proof arrived from another peer")
	}

	// Check that the signature carries the desired owner's key.
	key, err := s.GetSignature().ParsePubKey()
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, peer.ErrEmptyPeerID
	}
	signer, err := peer.IDFromPublicKey(key)
	if err != nil {
		return nil, err
	}
	if signer.String() != desiredOwner {
		return nil, errors.New("replica proof is signed by another peer")
	}

	// Check the signature of the encoded proof.
	valid, err := s.GetSignature().VerifyWithPublic(proofSignContext, key, s.GetProof())
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, errors.New("replica proof signature is invalid")
	}

	// Check that it names the object and the signer.
	proof := &SOReplicaProof{}
	if err := proof.UnmarshalVT(s.GetProof()); err != nil {
		return nil, err
	}
	if proof.GetSharedObjectId() != sharedObjectID || proof.GetPeerId() != desiredOwner {
		return nil, errors.New("replica proof names another object or peer")
	}
	return proof, nil
}
