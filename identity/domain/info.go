package identity_domain

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/identity"
)

// UnmarshalDomainInfo unmarshals a DomainInfo from a cursor.
// If empty, returns nil, nil
func UnmarshalDomainInfo(ctx context.Context, bcs *block.Cursor) (*DomainInfo, error) {
	return block.UnmarshalBlock[DomainInfo](ctx, bcs)
}

// FilterValue returns the value used to filter the list against this
// DomainInfo.
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

// _ is a type assertion
var _ block.Block = (*DomainInfo)(nil)
