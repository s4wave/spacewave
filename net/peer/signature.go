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
	if err := hashType.Validate(); err != nil {
		return nil, err
	}

	// prepend the encryption context and hash type
	signBody := bytes.Join([][]byte{
		[]byte(encContext),
		[]byte(strconv.Itoa(int(hashType))),
		hashData,
	}, []byte(" - SIGN - "))
	defer scrub.Scrub(signBody)

	sd, err := privKey.Sign(signBody)
	if err != nil {
		return nil, err
	}

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
	if err := s.GetHashType().Validate(); err != nil {
		return err
	}
	if len(s.GetSigData()) == 0 {
		return ErrSignatureInvalid
	}
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

	// hash the data
	dataHash, err := hash.Sum(ht, data)
	if err != nil {
		return false, err
	}
	defer scrub.Scrub(dataHash.Hash)

	// prepend the encryption context and hash type
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
	pubKey := s.GetPubKey()
	if len(pubKey) == 0 {
		return nil, nil
	}
	return crypto.UnmarshalPublicKey(pubKey)
}
