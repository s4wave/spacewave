package sobject

import (
	"encoding/binary"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/util/scrub"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/zeebo/blake3"
)

// baseCryptoContext is the base string for the crypto context.
var baseCryptoContext = "sobject 2024-05-22T20:10:42.613604Z shared object crypto ctx v1."

// BuildValidatorRootSignatureContext builds the context string for a validator signature on shared object root.
func BuildValidatorRootSignatureContext(sharedObjectID string, seqno uint64) string {
	var b strings.Builder
	b.WriteString(baseCryptoContext)
	b.WriteString("validator_root_signature ")
	b.WriteString(sharedObjectID)
	b.WriteString(" seqno ")
	b.WriteString(strconv.FormatUint(seqno, 10))
	return b.String()
}

// BuildSOOperationSignatureContext builds the context string for a participant signature on a shared object operation.
func BuildSOOperationSignatureContext(sharedObjectID string, peerID string, nonce uint64, localID string) string {
	var b strings.Builder
	b.WriteString(baseCryptoContext)
	b.WriteString("participant_operation_signature ")
	b.WriteString(sharedObjectID)
	b.WriteString(" peer ")
	b.WriteString(peerID)
	b.WriteString(" local-id ")
	b.WriteString(localID)
	hashNonce(&b, nonce)

	return b.String()
}

// hashNonce uses the contents of the string builder as a crypto context.
// hashes the nonce and appends it to sb.
func hashNonce(sb *strings.Builder, nonce uint64) {
	opNonceBytes := binary.LittleEndian.AppendUint64(nil, nonce)
	key := make([]byte, 32)
	blake3.DeriveKey(sb.String(), opNonceBytes, key)
	sb.WriteString(" op-nonce ")
	sb.WriteString(b58.Encode(key))
	scrub.Scrub(key)
}

// BuildSOOperationRejectionSignatureContext builds the context string for a validator signature on a shared object operation rejection.
func BuildSOOperationRejectionSignatureContext(sharedObjectID string, validatorPeerID, submitterPeerID string, opNonce uint64, localID string) string {
	var b strings.Builder
	b.WriteString(baseCryptoContext)
	b.WriteString("validator_operation_rejection_signature ")
	b.WriteString(sharedObjectID)
	b.WriteString(" validator ")
	b.WriteString(validatorPeerID)
	b.WriteString(" submitter ")
	b.WriteString(submitterPeerID)
	b.WriteString(" local-id ")
	b.WriteString(localID)
	hashNonce(&b, opNonce)

	return b.String()
}

// BuildSOOperationRejectionErrorDetailsEncContext builds the context string for error details on a shared object operation rejection.
func BuildSOOperationRejectionErrorDetailsContext(sharedObjectID string, validatorPeerID, submitterPeerID string, opNonce uint64, localID string) string {
	var b strings.Builder
	b.WriteString(baseCryptoContext)
	b.WriteString("validator_operation_rejection_error_details_enc ")
	b.WriteString(sharedObjectID)
	b.WriteString(" validator ")
	b.WriteString(validatorPeerID)
	b.WriteString(" submitter ")
	b.WriteString(submitterPeerID)
	b.WriteString(" local-id ")
	b.WriteString(localID)
	hashNonce(&b, opNonce)

	return b.String()
}

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

// BuildSOGrantEncContext builds the context string for a encrypt inner on a SOGrant.
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

// BuildSOClearOperationResultSignatureContext builds the context string for a signature on a clear operation result.
func BuildSOClearOperationResultSignatureContext(sharedObjectID string, peerID string, localID string) string {
	var b strings.Builder
	b.WriteString(baseCryptoContext)
	b.WriteString("clear_operation_result_signature ")
	b.WriteString(sharedObjectID)
	b.WriteString(" peer ")
	b.WriteString(peerID)
	b.WriteString(" local-id ")
	b.WriteString(localID)
	return b.String()
}

// ValidateSignatures validates the signatures on a SORoot.
// Returns the number of valid validator signatures and any error.
func (r *SORoot) ValidateSignatures(sharedObjectID string, participants []*SOParticipantConfig) (int, error) {
	if len(r.GetInner()) == 0 {
		return 0, ErrEmptyInnerData
	}

	// Validate account nonces are sorted
	if !slices.IsSortedFunc(r.GetAccountNonces(), func(a, b *SOAccountNonce) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	}) {
		return 0, errors.New("account nonces not sorted by peer_id")
	}

	signData, err := r.BuildSignatureData()
	if err != nil {
		return 0, err
	}
	defer scrub.Scrub(signData)

	// Track seen validator peer IDs to detect duplicates
	seenValidators := make(map[string]struct{})

	for i, sig := range r.GetValidatorSignatures() {
		pubKey, err := sig.ParsePubKey()
		if err != nil {
			return 0, err
		}
		if pubKey == nil {
			return 0, peer.ErrEmptyPeerID
		}

		peerID, err := peer.IDFromPublicKey(pubKey)
		if err != nil {
			return 0, errors.Wrapf(err, "validator_signatures[%d]", i)
		}

		// Check for duplicate validator signatures
		peerIDStr := peerID.String()
		if _, ok := seenValidators[peerIDStr]; ok {
			return 0, errors.Errorf("validator_signatures[%d]: duplicate validator signature for peer %s", i, peerIDStr)
		}
		seenValidators[peerIDStr] = struct{}{}

		var canValidate bool
		for _, p := range participants {
			if p.GetPeerId() == peerIDStr {
				canValidate = IsValidatorOrOwner(p.GetRole())
				break
			}
		}
		if !canValidate {
			return 0, errors.Errorf("validator_signatures[%d]: not a valid validator signature", i)
		}

		encContext := BuildValidatorRootSignatureContext(sharedObjectID, r.GetInnerSeqno())
		valid, err := sig.VerifyWithPublic(encContext, pubKey, signData)
		if err != nil {
			return 0, errors.Wrapf(err, "validator_signatures[%d]: failed to verify", i)
		}
		if !valid {
			return 0, errors.Errorf("validator_signatures[%d]: invalid signature", i)
		}
	}

	return len(seenValidators), nil
}

// CheckConsensusAcceptance checks whether the given number of valid signatures
// satisfies the consensus mode. Returns an error if consensus is not met.
func CheckConsensusAcceptance(mode SOConsensusMode, validSigs int) error {
	switch mode {
	case SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR:
		if validSigs < 1 {
			return ErrEmptyValidatorSignatures
		}
		return nil
	default:
		return errors.Errorf("unsupported consensus mode: %d", int32(mode))
	}
}

// EncryptSOGrant encrypts the inner data of a SOGrant.
// The privKey is also used to sign the inner data.
func EncryptSOGrant(privKey crypto.PrivKey, toPubKey crypto.PubKey, sharedObjectID string, nextInner *SOGrantInner) (*SOGrant, error) {
	// signer peer id
	signerPeerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	signerPeerIDStr := signerPeerID.String()

	// validate
	if err := nextInner.Validate(); err != nil {
		return nil, err
	}

	nextInnerData, err := nextInner.MarshalVT()
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(nextInnerData)
	if len(nextInnerData) == 0 {
		// enforce not zero (although validate also checks this)
		return nil, ErrEmptyInnerData
	}

	// to peer id
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

// ValidateSignature validates the signature on a SOOperation.
func (op *SOOperation) ValidateSignature(sharedObjectID string, participants []*SOParticipantConfig) error {
	if len(op.GetInner()) == 0 {
		return ErrEmptyInnerData
	}

	sig := op.GetSignature()
	pubKey, err := sig.ParsePubKey()
	if err != nil {
		return err
	}
	if pubKey == nil {
		return peer.ErrEmptyPeerID
	}

	peerID, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return errors.Wrap(err, "invalid peer ID in signature")
	}

	peerIDStr := peerID.String()

	var seen bool
	for _, p := range participants {
		if p.GetPeerId() == peerIDStr && CanWriteOps(p.GetRole()) {
			seen = true
			break
		}
	}
	if !seen {
		return ErrNotParticipant
	}

	// Unmarshal the inner data to get the nonce
	inner := &SOOperationInner{}
	if err := inner.UnmarshalVT(op.GetInner()); err != nil {
		return errors.Wrap(err, "failed to unmarshal inner data")
	}

	encContext := BuildSOOperationSignatureContext(sharedObjectID, peerIDStr, inner.GetNonce(), inner.GetLocalId())
	valid, err := sig.VerifyWithPublic(encContext, pubKey, op.GetInner())
	if err != nil {
		return errors.Wrap(err, "failed to verify signature")
	}
	if !valid {
		return peer.ErrSignatureInvalid
	}

	return nil
}

// ValidateSignature validates the signature on a SOGrant.
func (g *SOGrant) ValidateSignature(sharedObjectID string, participants []*SOParticipantConfig) error {
	if len(g.GetInnerData()) == 0 {
		return ErrEmptyInnerData
	}

	sig := g.GetSignature()
	pubKey, err := sig.ParsePubKey()
	if err != nil {
		return err
	}
	if pubKey == nil {
		return peer.ErrEmptyPeerID
	}

	peerID, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return err
	}

	peerIDStr := peerID.String()
	var canValidate bool
	for _, p := range participants {
		if p.GetPeerId() != peerIDStr {
			continue
		}
		if IsValidatorOrOwner(p.GetRole()) {
			canValidate = true
			break
		}
		if peerIDStr == g.GetPeerId() && CanReadState(p.GetRole()) {
			canValidate = true
			break
		}
	}
	if !canValidate {
		return ErrEmptyValidatorSignatures
	}

	encContext := BuildSOGrantSignatureContext(sharedObjectID, peerIDStr, g.GetPeerId())
	valid, err := sig.VerifyWithPublic(encContext, pubKey, g.GetInnerData())
	if err != nil {
		return errors.Wrap(err, "failed to verify signature")
	}
	if !valid {
		return peer.ErrSignatureInvalid
	}

	return nil
}

// ValidateSignature validates the signature on a SOOperationRejection.
func (r *SOOperationRejection) ValidateSignature(sharedObjectID string, participants []*SOParticipantConfig) (*SOOperationRejectionInner, error) {
	if len(r.GetInner()) == 0 {
		return nil, ErrEmptyInnerData
	}

	sig := r.GetSignature()
	pubKey, err := sig.ParsePubKey()
	if err != nil {
		return nil, err
	}
	if pubKey == nil {
		return nil, peer.ErrEmptyPeerID
	}

	validatorPeerID, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return nil, err
	}

	validatorPeerIDStr := validatorPeerID.String()
	var canValidate bool
	for _, p := range participants {
		if p.GetPeerId() == validatorPeerIDStr && IsValidatorOrOwner(p.GetRole()) {
			canValidate = true
			break
		}
	}
	if !canValidate {
		return nil, ErrEmptyValidatorSignatures
	}

	// Parse the inner data
	inner := &SOOperationRejectionInner{}
	if err := inner.UnmarshalVT(r.GetInner()); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal inner data")
	}
	if err := inner.Validate(); err != nil {
		return nil, err
	}

	encContext := BuildSOOperationRejectionSignatureContext(sharedObjectID, validatorPeerIDStr, inner.GetPeerId(), inner.GetOpNonce(), inner.GetLocalId())
	valid, err := sig.VerifyWithPublic(encContext, pubKey, r.GetInner())
	if err != nil {
		inner.Reset()
		return nil, err
	}
	if !valid {
		inner.Reset()
		return nil, peer.ErrSignatureInvalid
	}

	return inner, nil
}
