package forge_lib_kvtx

import (
	"context"

	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
)

// NewConfigInputBlock constructs a new ConfigInput block.
func NewConfigInputBlock() block.Block {
	return &ConfigInput{}
}

// FetchConfigInput fetches the ConfigInput block from the Value.
func FetchConfigInput(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	val *forge_value.Value,
) (*ConfigInput, error) {
	objRef, err := val.ToBucketRef()
	if err != nil {
		return nil, err
	}
	var confInput *ConfigInput
	err = handle.AccessStorage(ctx, objRef, func(bls *bucket_lookup.Cursor) error {
		_, bcs := bls.BuildTransaction(nil)
		var berr error
		confInput, berr = UnmarshalConfigInput(bcs)
		return berr
	})
	return confInput, err
}

// UnmarshalConfigInput unmarshals a config input block from the cursor.
func UnmarshalConfigInput(bcs *block.Cursor) (*ConfigInput, error) {
	vi, err := bcs.Unmarshal(NewConfigInputBlock)
	if err != nil {
		return nil, err
	}
	if vi == nil {
		return nil, nil
	}
	b, ok := vi.(*ConfigInput)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return b, nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *ConfigInput) MarshalBlock() ([]byte, error) {
	return b.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *ConfigInput) UnmarshalBlock(data []byte) error {
	return b.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*ConfigInput)(nil))
