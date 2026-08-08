package s4wave_apt

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// AptPackageTypeID is the type identifier for AptPackage objects.
const AptPackageTypeID = "spacewave-vm/apt/package"

// NewAptPackageBlock constructs a new AptPackage block.
func NewAptPackageBlock() block.Block {
	return &AptPackage{}
}

// GetBlockTypeId returns the block type identifier.
func (p *AptPackage) GetBlockTypeId() string {
	return AptPackageTypeID
}

// MarshalBlock marshals the block to binary.
func (p *AptPackage) MarshalBlock() ([]byte, error) {
	return p.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (p *AptPackage) UnmarshalBlock(data []byte) error {
	return p.UnmarshalVT(data)
}

// Validate performs cursory checks on the AptPackage.
func (p *AptPackage) Validate() error {
	if err := p.GetState().Validate(); err != nil {
		return err
	}
	if p.GetName() == "" {
		return errors.New("name is required")
	}
	if p.GetVersion() == "" {
		return errors.New("version is required")
	}
	if p.GetArchitecture() == "" {
		return errors.New("architecture is required")
	}
	if p.GetState() != AptPackageState_AptPackageState_IMPORTING && p.GetDebRef().GetEmpty() {
		return errors.New("deb_ref is required")
	}
	if !p.GetDebRef().GetEmpty() {
		if err := p.GetDebRef().Validate(false); err != nil {
			return err
		}
	}
	for _, checksum := range p.GetChecksums() {
		if checksum.GetAlgorithm() == "" {
			return errors.New("checksum algorithm is required")
		}
		if checksum.GetHex() == "" {
			return errors.New("checksum hex is required")
		}
	}
	return nil
}

// _ is a type assertion
var _ block.Block = (*AptPackage)(nil)
