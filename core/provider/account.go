package provider

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/util/labels"
)

// NewProviderAccountInfoBlock constructs a new ProviderAccountInfo block.
func NewProviderAccountInfoBlock() block.Block {
	return &ProviderAccountInfo{}
}

// ValidateProviderAccountID validates a provider identifier.
func ValidateProviderAccountID(id string) error {
	if id == "" {
		return ErrEmptyProviderAccountID
	}
	if err := labels.ValidateDNSLabel(id); err != nil {
		return errors.Wrap(err, "provider account id")
	}
	return nil
}

// UnmarshalProviderAccountInfo unmarshals a ProviderAccountInfo from a block cursor.
func UnmarshalProviderAccountInfo(ctx context.Context, bcs *block.Cursor) (*ProviderAccountInfo, error) {
	return block.UnmarshalBlock[*ProviderAccountInfo](ctx, bcs, NewProviderAccountInfoBlock)
}

// Validate validates the ProviderAccountInfo.
func (i *ProviderAccountInfo) Validate() error {
	if err := ValidateProviderID(i.GetProviderId()); err != nil {
		return err
	}
	if err := ValidateProviderAccountID(i.GetProviderAccountId()); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (i *ProviderAccountInfo) MarshalBlock() ([]byte, error) {
	return i.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (i *ProviderAccountInfo) UnmarshalBlock(data []byte) error {
	return i.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*ProviderAccountInfo)(nil))
