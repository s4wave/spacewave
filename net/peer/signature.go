package peer

import (
	"bytes"
	"strconv"

	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
)

// NewSignature constructs a signature.
//
// encContext strings must be hardcoded constants, and the recommended
// format is "[application] [commit timestamp] [purpose]", e.g.,
// "example.com 2019-12-25 16:18:03 session tokens v1".
func NewSignature(
	encContext string,
	privKey crypto.PrivKey,
	hashType hash.HashType,
	data []byte,
	inclPubKey bool,
) (*Signature, error) {
	// Hash the data before constructing the signature body.
	h, err := hash.Sum(hashType, data)
	if err != nil {
		return nil, err
	}
	return NewSignatureWithHashedData(
		encContext,
		privKey,
		hashType,
		h.GetHash(),
		inclPubKey,
	)
}

// NewSignatureWithHashedData builds a new signature with already-hashed data.
// Skips the hash step.
//
// encContext strings must be hardcoded constants, and the recommended
// format is "[application] [commit timestamp] [purpose]", e.g.,
// "example.com 2019-12-25 16:18:03 session tokens v1".
func NewSignatureWithHashedData(
	encContext string,
	privKey crypto.PrivKey,
	hashType hash.HashType,
	hashData []byte,
	inclPubKey bool,
) (*Signature, error) {
	// Validate the requested hash algorithm.
	if err := hashType.Validate(); err != nil {
		return nil, err
	}

	// Build the signed body from the context, hash type, and digest.
	signBody := bytes.Join([][]byte{
		[]byte(encContext),
		[]byte(strconv.Itoa(int(hashType))),
		hashData,
	}, []byte(" - SIGN - "))
	defer scrub.Scrub(signBody)

	// Sign the constructed body with the private key.
	sd, err := privKey.Sign(signBody)
	if err != nil {
		return nil, err
	}

	// Assemble the signature and optionally include the public key.
	s := &Signature{HashType: hashType, SigData: sd}
	if inclPubKey {
		pkey, err := crypto.MarshalPublicKey(privKey.GetPublic())
		if err != nil {
			return nil, err
		}
		s.PubKey = pkey
	}

	return s, nil
}

// Validate checks the signature object (but not the signature itself).
func (s *Signature) Validate() error {
	// Validate the hash type and signature bytes.
	if err := s.GetHashType().Validate(); err != nil {
		return err
	}
	if len(s.GetSigData()) == 0 {
		return ErrSignatureInvalid
	}

	// Validate the embedded public key when present.
	if len(s.GetPubKey()) != 0 {
		if _, err := s.ParsePubKey(); err != nil {
			return errors.Wrap(err, "pub_key")
		}
	}
	return nil
}

// VerifyWithPublic checks a signature with a public key, hashing the data.
// Returns ok and any error interpeting the signature.
//
// encContext must match the context used when creating the signature.
func (s *Signature) VerifyWithPublic(encContext string, pubKey crypto.PubKey, data []byte) (bool, error) {
	// Validate the signature metadata before hashing the data.
	ht := s.GetHashType()
	if ht == hash.HashType_HashType_UNKNOWN {
		return false, errors.New("hash type missing")
	}
	if len(s.GetSigData()) == 0 {
		return false, errors.New("signature empty")
	}
	if err := ht.Validate(); err != nil {
		return false, err
	}

	// Hash the data with the signature's declared algorithm.
	dataHash, err := hash.Sum(ht, data)
	if err != nil {
		return false, err
	}
	defer scrub.Scrub(dataHash.Hash)

	// Build the signed body from the context, hash type, and digest.
	signBody := bytes.Join([][]byte{
		[]byte(encContext),
		[]byte(strconv.Itoa(int(ht))),
		dataHash.Hash,
	}, []byte(" - SIGN - "))
	defer scrub.Scrub(signBody)

	return pubKey.Verify(signBody, s.GetSigData())
}

// ParsePubKey parses the incldued public key.
// Returns nil, nil if the pub key field was not set.
func (s *Signature) ParsePubKey() (crypto.PubKey, error) {
	// Return no key when the signature omitted its public key.
	pubKey := s.GetPubKey()
	if len(pubKey) == 0 {
		return nil, nil
	}
	return crypto.UnmarshalPublicKey(pubKey)
}
