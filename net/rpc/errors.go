package bifrost_rpc

import "errors"

// ErrServiceClientUnavailable is returned if no clients are available for a service.
var ErrServiceClientUnavailable = errors.New("no client available for that service")
