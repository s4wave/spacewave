package forge_target

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// IsNil checks if the object is nil.
func (e *Exec) IsNil() bool {
	return e == nil
}

// Validate cursorily validates the Exec.
func (e *Exec) Validate() error {
	if e.GetDisable() {
		return nil
	}
	if err := e.GetController().Validate(); err != nil {
		return errors.Wrap(err, "controller")
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block = (*Exec)(nil)
)
