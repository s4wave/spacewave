package session

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// NewSessionRefBlock constructs a new SessionRef block.
func NewSessionRefBlock() block.Block {
	return &SessionRef{}
}

// UnmarshalSessionRef unmarshals a SessionRef from a block cursor.
func UnmarshalSessionRef(ctx context.Context, bcs *block.Cursor) (*SessionRef, error) {
	return block.UnmarshalBlock[*SessionRef](ctx, bcs, NewSessionRefBlock)
}

// Validate validates the shared object ref.
func (i *SessionRef) Validate() error {
	if err := i.GetProviderResourceRef().Validate(); err != nil {
		return err
	}
	return nil
}

// GetLogger adds debug values to the logger.
func (i *SessionRef) GetLogger(le *logrus.Entry) *logrus.Entry {
	return i.
		GetProviderResourceRef().
		GetLogger(le)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (i *SessionRef) MarshalBlock() ([]byte, error) {
	return i.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (i *SessionRef) UnmarshalBlock(data []byte) error {
	return i.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*SessionRef)(nil))
