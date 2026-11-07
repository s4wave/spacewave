package sobject

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// baseCryptoContext is the base string for the crypto context.
var baseCryptoContext = "sobject 2024-05-22T20:10:42.613604Z shared object crypto ctx v1."

// BuildSOGrantSignatureContext builds the context string for a signature on a SOGrant.
func BuildSOGrantSignatureContext(sharedObjectID string, signerPeerID, recipientPeerID string) string {
	return baseCryptoContext + "grant_signature " + sharedObjectID + " signer " + signerPeerID + " recipient " + recipientPeerID
}

// BuildSOGrantEncContext builds the context for encrypting a SOGrant's inner data.
func BuildSOGrantEncContext(sharedObjectID string, fromPeerID string, recipientPeerID string) string {
	return baseCryptoContext + "grant_enc_inner " + sharedObjectID + " signer " + fromPeerID + " recipient " + recipientPeerID
}

// domainHash returns the SHA-256 digest of domain, the big-endian length of
// inner, and inner, so a digest of one record kind never equals another's.
func domainHash(domain string, inner []byte) []byte {
	// Hash the length-framed body without copying it.
	digest := sha256.New()
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(inner)))
	digest.Write([]byte(domain))
	digest.Write(length[:])
	digest.Write(inner)
	return digest.Sum(nil)
}

// signerOf returns the public key sig carries and the peer it identifies. It
// does not verify the signature.
func signerOf(sig *peer.Signature) (crypto.PubKey, peer.ID, error) {
	// Require the public key the signature carries.
	pubKey, err := sig.ParsePubKey()
	if err != nil {
		return nil, "", err
	}
	if pubKey == nil {
		return nil, "", peer.ErrEmptyPeerID
	}

	// Derive the signing peer from that key.
	signer, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return nil, "", errors.Wrap(err, "invalid peer ID in signature")
	}
	return pubKey, signer, nil
}

// verifySignature checks that sig covers exactly data under encContext and
// returns the signer.
func verifySignature(sig *peer.Signature, encContext string, data []byte) (peer.ID, error) {
	// Identify the signer.
	pubKey, signer, err := signerOf(sig)
	if err != nil {
		return "", err
	}

	// Check the signature over data.
	valid, err := sig.VerifyWithPublic(encContext, pubKey, data)
	if err != nil {
		return "", errors.Wrap(err, "failed to verify signature")
	}
	if !valid {
		return "", peer.ErrSignatureInvalid
	}
	return signer, nil
}

// EncryptSOGrant encrypts the inner data of a SOGrant.
// The privKey is also used to sign the inner data.
func EncryptSOGrant(privKey crypto.PrivKey, toPubKey crypto.PubKey, sharedObjectID string, nextInner *SOGrantInner) (*SOGrant, error) {
	// Bind the grant to its signer and recipient.
	signerPeerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	signerPeerIDStr := signerPeerID.String()

	// Serialize a valid grant before encrypting its inner data.
	if err := nextInner.Validate(); err != nil {
		return nil, err
	}

	nextInnerData, err := nextInner.MarshalVT()
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(nextInnerData)
	if len(nextInnerData) == 0 {
		return nil, ErrEmptyInnerData
	}

	toPeerID, err := peer.IDFromPublicKey(toPubKey)
	if err != nil {
		return nil, err
	}
	toPeerIDStr := toPeerID.String()

	innerDataEnc, err := peer.EncryptToPubKey(
		toPubKey,
		BuildSOGrantEncContext(sharedObjectID, signerPeerIDStr, toPeerIDStr),
		nextInnerData,
	)
	if err != nil {
		return nil, err
	}

	sig, err := peer.NewSignature(
		BuildSOGrantSignatureContext(sharedObjectID, signerPeerIDStr, toPeerIDStr),
		privKey,
		hash.RecommendedHashType,
		innerDataEnc,
		true,
	)
	if err != nil {
		scrub.Scrub(innerDataEnc)
		return nil, err
	}

	return &SOGrant{
		PeerId:    toPeerIDStr,
		InnerData: innerDataEnc,
		Signature: sig,
	}, nil
}

// DecryptInnerData decrypts the inner data of a SOGrant.
func (g *SOGrant) DecryptInnerData(privKey crypto.PrivKey, sharedObjectID string) (*SOGrantInner, error) {
	// Decrypt under the context bound to the grant's signer and recipient.
	_, signer, err := signerOf(g.GetSignature())
	if err != nil {
		return nil, err
	}
	innerDataDec, err := peer.DecryptWithPrivKey(
		privKey,
		BuildSOGrantEncContext(sharedObjectID, signer.String(), g.GetPeerId()),
		g.GetInnerData(),
	)
	if err != nil {
		return nil, err
	}

	// Decode the plaintext, then scrub it.
	innerDataObj := &SOGrantInner{}
	err = innerDataObj.UnmarshalVT(innerDataDec)
	scrub.Scrub(innerDataDec)
	if err != nil {
		return nil, err
	}

	return innerDataObj, nil
}

// Verify authenticates the grant's signature and returns its signer. It does
// not check that the signer may issue the grant; ValidateSignature does.
func (g *SOGrant) Verify(sharedObjectID string) (string, error) {
	// Identify the signer.
	if len(g.GetInnerData()) == 0 {
		return "", ErrEmptyInnerData
	}
	sig := g.GetSignature()
	pubKey, signer, err := signerOf(sig)
	if err != nil {
		return "", err
	}

	// The signature binds the signer and recipient to the encrypted key.
	encContext := BuildSOGrantSignatureContext(sharedObjectID, signer.String(), g.GetPeerId())
	valid, err := sig.VerifyWithPublic(encContext, pubKey, g.GetInnerData())
	if err != nil {
		return "", errors.Wrap(err, "failed to verify signature")
	}
	if !valid {
		return "", peer.ErrSignatureInvalid
	}
	return signer.String(), nil
}

// ValidateSignature authenticates the grant and checks that an owner of cfg,
// a voter under its group control, or the grant's reading recipient signed
// it. Under group control any voter hands the key to a member the group
// admitted.
func (g *SOGrant) ValidateSignature(sharedObjectID string, cfg *SharedObjectConfig) error {
	signer, err := g.Verify(sharedObjectID)
	if err != nil {
		return err
	}
	return g.validateSigner(signer, cfg)
}

// validateSigner checks that signer, which Verify authenticated, is an owner
// of cfg, a voter under its group control, or the grant's reading recipient.
func (g *SOGrant) validateSigner(signer string, cfg *SharedObjectConfig) error {
	for _, p := range cfg.GetParticipants() {
		if p.GetPeerId() != signer {
			continue
		}
		if IsOwner(p.GetRole()) || cfg.VotingWeight(signer) != 0 || (signer == g.GetPeerId() && CanReadState(p.GetRole())) {
			return nil
		}
	}
	return errors.New("grant is not signed by an owner, a voter or its recipient")
}
