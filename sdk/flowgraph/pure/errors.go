package flowgraph_pure

import "github.com/pkg/errors"

// ErrEmptyFunctionID is returned when a lookup names no function.
var ErrEmptyFunctionID = errors.New("pure function ID is empty")
