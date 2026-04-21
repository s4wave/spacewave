package block

import (
	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/net/hash"
)

// Validate validates the put opts.
func (o *PutOpts) Validate() error {
	if o == nil {
		return nil
	}
	if o.GetHashType() != 0 {
		if err := o.GetHashType().Validate(); err != nil {
			return err
		}
	}
	return nil
}

// SelectHashType selects the hash type to use for the operation.
// The given hash type should be the default value to use.
func (o *PutOpts) SelectHashType(defHashType hash.HashType) hash.HashType {
	forceHashType := o.GetForceBlockRef().GetHash().GetHashType()
	if forceHashType != 0 {
		return forceHashType
	}
	if selHashType := o.GetHashType(); selHashType != 0 {
		return selHashType
	}
	if defHashType != 0 {
		return defHashType
	}
	return DefaultHashType
}

// MarshalString marshals the put opts to b58 string.
func (o *PutOpts) MarshalString() string {
	return o.MarshalB58()
}

// MarshalB58 marshals the put opts to a base58 string form.
func (o *PutOpts) MarshalB58() string {
	if o == nil {
		return ""
	}
	dat, err := o.MarshalVT()
	if err != nil {
		return ""
	}
	return b58.Encode(dat)
}

// UnmarshalB58 unmarshals the put opts from base58 string form.
func (o *PutOpts) UnmarshalB58(ref string) error {
	o.Reset()
	if ref == "" {
		return nil
	}

	dat, err := b58.Decode(ref)
	if err != nil {
		return err
	}
	return o.UnmarshalVT(dat)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *PutOpts) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *PutOpts) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ Block = ((*PutOpts)(nil))
