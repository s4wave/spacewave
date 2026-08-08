// Package s4wave_apt implements apt repository block types and world operations.
package s4wave_apt

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// AptRepositoryTypeID is the type identifier for AptRepository objects.
const AptRepositoryTypeID = "spacewave-vm/apt/repository"

// NewAptRepositoryBlock constructs a new AptRepository block.
func NewAptRepositoryBlock() block.Block {
	return &AptRepository{}
}

// GetBlockTypeId returns the block type identifier.
func (r *AptRepository) GetBlockTypeId() string {
	return AptRepositoryTypeID
}

// MarshalBlock marshals the block to binary.
func (r *AptRepository) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (r *AptRepository) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// Validate performs cursory checks on the AptRepository.
func (r *AptRepository) Validate() error {
	if err := r.GetState().Validate(); err != nil {
		return err
	}
	if r.GetDistribution() == "" {
		return errors.New("distribution is required")
	}
	if len(r.GetComponents()) == 0 {
		return errors.New("components are required")
	}
	if len(r.GetArchitectures()) == 0 {
		return errors.New("architectures are required")
	}
	if r.GetState() == AptRepositoryState_AptRepositoryState_READY && r.GetIndexRef().GetEmpty() {
		return errors.New("index_ref is required when repository is ready")
	}
	if !r.GetIndexRef().GetEmpty() {
		return r.GetIndexRef().Validate()
	}
	return nil
}

// _ is a type assertion
var _ block.Block = (*AptRepository)(nil)
