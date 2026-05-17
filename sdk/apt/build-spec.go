package s4wave_apt

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// AptBuildSpecTypeID is the type identifier for AptBuildSpec objects.
const AptBuildSpecTypeID = "spacewave-vm/apt/build-spec"

// NewAptBuildSpecBlock constructs a new AptBuildSpec block.
func NewAptBuildSpecBlock() block.Block {
	return &AptBuildSpec{}
}

// GetBlockTypeId returns the block type identifier.
func (s *AptBuildSpec) GetBlockTypeId() string {
	return AptBuildSpecTypeID
}

// MarshalBlock marshals the block to binary.
func (s *AptBuildSpec) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (s *AptBuildSpec) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// Validate performs cursory checks on the AptBuildSpec.
func (s *AptBuildSpec) Validate() error {
	if s.GetSourcePackage() == "" {
		return errors.New("source_package is required")
	}
	if s.GetSourceRef().GetEmpty() {
		return errors.New("source_ref is required")
	}
	if err := s.GetSourceRef().Validate(); err != nil {
		return err
	}
	if len(s.GetArchitectures()) == 0 {
		return errors.New("architectures are required")
	}
	for key := range s.GetBuildConfig().GetEnv() {
		if key == "" {
			return errors.New("build_config env key is required")
		}
	}
	return nil
}

// _ is a type assertion
var _ block.Block = ((*AptBuildSpec)(nil))
