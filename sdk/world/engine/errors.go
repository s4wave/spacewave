package sdk_world_engine

import "errors"

// ErrRemoteCayleyGraphUnsupported is returned when a remote SDK world is
// asked for a local Cayley handle.
var ErrRemoteCayleyGraphUnsupported = errors.New("remote Cayley graph access is unsupported; use bounded graph RPCs")
