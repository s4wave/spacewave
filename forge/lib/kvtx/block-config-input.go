package forge_lib_kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
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
		confInput, berr = UnmarshalConfigInput(ctx, bcs)
		return berr
	})
	return confInput, err
}

// UnmarshalConfigInput unmarshals a config input block from the cursor.
func UnmarshalConfigInput(ctx context.Context, bcs *block.Cursor) (*ConfigInput, error) {
	return block.UnmarshalBlock[*ConfigInput](ctx, bcs, NewConfigInputBlock)
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
