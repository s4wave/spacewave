package volume_scoped

import "github.com/pkg/errors"

// ErrRefused is returned when a scoped volume view refuses an operation
// because it reaches data outside the view.
var ErrRefused = errors.New("volume view: operation refused")
