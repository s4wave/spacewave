package resource_root

import (
	"context"

	"github.com/s4wave/spacewave/net/hash"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// MarshalHash marshals a Hash to a base58 string.
func (s *CoreRootServer) MarshalHash(
	ctx context.Context,
	req *s4wave_root.MarshalHashRequest,
) (*s4wave_root.MarshalHashResponse, error) {
	h := req.GetHash()
	if h == nil || h.IsEmpty() {
		return &s4wave_root.MarshalHashResponse{HashStr: ""}, nil
	}

	hashStr := h.MarshalString()
	return &s4wave_root.MarshalHashResponse{HashStr: hashStr}, nil
}

// ParseHash parses a Hash from a base58 string.
func (s *CoreRootServer) ParseHash(
	ctx context.Context,
	req *s4wave_root.ParseHashRequest,
) (*s4wave_root.ParseHashResponse, error) {
	hashStr := req.GetHashStr()
	if hashStr == "" {
		return &s4wave_root.ParseHashResponse{Hash: nil}, nil
	}

	h := &hash.Hash{}
	if err := h.ParseFromB58(hashStr); err != nil {
		return nil, err
	}

	return &s4wave_root.ParseHashResponse{Hash: h}, nil
}

// HashSum computes a hash of the given data with the specified hash type.
func (s *CoreRootServer) HashSum(
	ctx context.Context,
	req *s4wave_root.HashSumRequest,
) (*s4wave_root.HashSumResponse, error) {
	hashType := req.GetHashType()
	data := req.GetData()

	h, err := hash.Sum(hashType, data)
	if err != nil {
		return nil, err
	}

	return &s4wave_root.HashSumResponse{Hash: h}, nil
}

// HashValidate validates a hash object.
func (s *CoreRootServer) HashValidate(
	ctx context.Context,
	req *s4wave_root.HashValidateRequest,
) (*s4wave_root.HashValidateResponse, error) {
	h := req.GetHash()
	if h == nil {
		return &s4wave_root.HashValidateResponse{
			Valid: false,
			Error: "hash is nil",
		}, nil
	}

	if err := h.Validate(); err != nil {
		return &s4wave_root.HashValidateResponse{
			Valid: false,
			Error: err.Error(),
		}, nil
	}

	return &s4wave_root.HashValidateResponse{Valid: true}, nil
}
