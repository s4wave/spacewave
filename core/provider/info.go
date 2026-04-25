package provider

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/util/labels"
)

// NewProviderInfoBlock constructs a new ProviderInfo block.
func NewProviderInfoBlock() block.Block {
	return &ProviderInfo{}
}

// ValidateProviderID validates a provider identifier.
func ValidateProviderID(id string) error {
	if id == "" {
		return ErrEmptyProviderID
	}
	if err := labels.ValidateDNSLabel(id); err != nil {
		return errors.Wrap(err, "provider id")
	}
	return nil
}

// UnmarshalProviderInfo unmarshals a ProviderInfo from a block cursor.
func UnmarshalProviderInfo(ctx context.Context, bcs *block.Cursor) (*ProviderInfo, error) {
	return block.UnmarshalBlock[*ProviderInfo](ctx, bcs, NewProviderInfoBlock)
}

// Validate validates the ProviderInfo.
func (i *ProviderInfo) Validate() error {
	if err := ValidateProviderID(i.GetProviderId()); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (i *ProviderInfo) MarshalBlock() ([]byte, error) {
	return i.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (i *ProviderInfo) UnmarshalBlock(data []byte) error {
	return i.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*ProviderInfo)(nil))
