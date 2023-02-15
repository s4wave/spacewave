package identity_domain

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/identity"
	"github.com/pkg/errors"
)

// NewDomainInfoBlock constructs a new Entity block
func NewDomainInfoBlock() block.Block {
	return &DomainInfo{}
}

// UnmarshalDomainInfo unmarshals a DomainInfo from a cursor.
// If empty, returns nil, nil
func UnmarshalDomainInfo(bcs *block.Cursor) (*DomainInfo, error) {
	return block.UnmarshalBlock[*DomainInfo](bcs, NewDomainInfoBlock)
}

// Filter value is the value we use when filtering against this item when
// we're filtering the list.
func (d *DomainInfo) FilterValue() string {
	return d.GetName()
}

// Validate validates the domain info.
func (d *DomainInfo) Validate() error {
	// note: allow empty domain id
	if len(d.GetDomainId()) != 0 {
		if err := identity.ValidateDomainID(d.GetDomainId()); err != nil {
			return errors.Wrap(err, "domain_id")
		}
	}
	if len(d.GetName()) == 0 {
		return errors.New("name cannot be empty")
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
