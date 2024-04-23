package identity_domain_service

import (
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// NewLookupEntityReq builds a new LookupEntityReq.
//
// if nonce is empty, generates randomly.
// if ts is nil, uses now()
func NewLookupEntityReq(
	domainID, entityID string,
	ts *timestamp.Timestamp,
	nonce uint64,
) (*LookupEntityReq, error) {
	// fill
	if nonce == 0 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}
		nonce = binary.LittleEndian.Uint64(b[:])
	}
	if ts == nil {
		ts = timestamp.Now()
	}
	return &LookupEntityReq{
		Identifier: &EntityLookupIdentifier{
			DomainId: domainID,
			EntityId: entityID,
		},
		Timestamp: ts,
		Nonce:     nonce,
	}, nil
}

// Validate checks the request.
func (r *LookupEntityReq) Validate() error {
	if err := r.GetIdentifier().Validate(); err != nil {
		return err
	}
	if err := r.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if r.GetNonce() == 0 {
		return errors.New("nonce must be a non-zero uint64")
	}
	return nil
}

// CheckTimestamp checks if the timestamp is within range.
func (r *LookupEntityReq) CheckTimestamp(now time.Time) error {
	// assert timestamp is within last 5 mins
	reqTs := r.GetTimestamp().ToTime()
	reqTsDiff := now.Sub(reqTs)
	if reqTsDiff > time.Minute*5 || reqTsDiff < -1*time.Second*30 {
		return errors.Errorf(
			"invalid timestamp clock skew: %s at %s",
			reqTs.String(),
			reqTsDiff.String(),
		)
	}
	return nil
}

// SignReq signs the request to a SignedMsg.
func (r *LookupEntityReq) SignReq(privKey crypto.PrivKey) (*peer.SignedMsg, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}

	dat, err := r.MarshalBlock()
	if err != nil {
		return nil, err
	}
	return peer.NewSignedMsg(privKey, hash.RecommendedHashType, dat)
}

// UnmarshalFrom attempts to unmarshal the request from the SignedMsg.
func (r *LookupEntityReq) UnmarshalFrom(req *peer.SignedMsg) (crypto.PubKey, error) {
	pubKey, _, err := req.ExtractAndVerify()
	if err != nil {
		return nil, err
	}
	if err := r.UnmarshalBlock(req.GetData()); err != nil {
		return pubKey, err
	}
	return pubKey, req.Validate()
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *LookupEntityReq) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *LookupEntityReq) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

var _ block.Block = ((*LookupEntityReq)(nil))
