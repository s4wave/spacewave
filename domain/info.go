package identity_domain

import (
	"errors"

	"github.com/aperturerobotics/hydra/block"
)

// NewDomainInfoBlock constructs a new Entity block
func NewDomainInfoBlock() block.Block {
	return &DomainInfo{}
}

// UnmarshalDomainInfo unmarshals a DomainInfo from a cursor.
// If empty, returns nil, nil
func UnmarshalDomainInfo(bcs *block.Cursor) (*DomainInfo, error) {
	if bcs == nil {
		return nil, nil
	}
	blk, err := bcs.Unmarshal(NewDomainInfoBlock)
	if err != nil {
		return nil, err
	}
	if blk == nil {
		return nil, nil
	}
	bv, ok := blk.(*DomainInfo)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return bv, nil
}

// Filter value is the value we use when filtering against this item when
// we're filtering the list.
func (d *DomainInfo) FilterValue() string {
	return d.GetName()
}

// Validate validates the domain info.
func (d *DomainInfo) Validate() error {
	// note: allow empty domain id
	if len(d.GetName()) == 0 {
		return errors.New("domain info: name cannot be empty")
	}
	return nil
}

// Clone clones the domain info.
func (d *DomainInfo) Clone() *DomainInfo {
	if d == nil {
		return nil
	}
	return &DomainInfo{
		DomainId:    d.GetDomainId(),
		Name:        d.GetName(),
		Description: d.GetDescription(),
	}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (d *DomainInfo) MarshalBlock() ([]byte, error) {
	return d.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (d *DomainInfo) UnmarshalBlock(data []byte) error {
	return d.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*DomainInfo)(nil))
