package sobject

import (
	"strings"

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
	var b strings.Builder
	b.WriteString(baseCryptoContext)
	b.WriteString("grant_signature ")
	b.WriteString(sharedObjectID)
	b.WriteString(" signer ")
	b.WriteString(signerPeerID)
	b.WriteString(" recipient ")
	b.WriteString(recipientPeerID)
	return b.String()
}

// BuildSOGrantEncContext builds the context for encrypting a SOGrant's inner data.
func BuildSOGrantEncContext(sharedObjectID string, fromPeerID string, recipientPeerID string) string {
	var b strings.Builder
	b.WriteString(baseCryptoContext)
	b.WriteString("grant_enc_inner ")
	b.WriteString(sharedObjectID)
	b.WriteString(" signer ")
	b.WriteString(fromPeerID)
	b.WriteString(" recipient ")
	b.WriteString(recipientPeerID)
	return b.String()
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
	signerPubKey, err := g.GetSignature().ParsePubKey()
	if err != nil {
		return nil, err
	}
	if signerPubKey == nil {
		return nil, peer.ErrEmptyPeerID
	}

	signerPeerID, err := peer.IDFromPublicKey(signerPubKey)
	if err != nil {
		return nil, err
	}
	signerPeerIDStr := signerPeerID.String()

	innerDataDec, err := peer.DecryptWithPrivKey(
		privKey,
		BuildSOGrantEncContext(sharedObjectID, signerPeerIDStr, g.GetPeerId()),
		g.GetInnerData(),
	)
	if err != nil {
		return nil, err
	}

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
	pubKey, err := sig.ParsePubKey()
	if err != nil {
		return "", err
	}
	if pubKey == nil {
		return "", peer.ErrEmptyPeerID
	}
	signer, err := peer.IDFromPublicKey(pubKey)
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
