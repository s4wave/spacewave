//go:build js

package metashard

import "github.com/pkg/errors"

// ErrReadOnly is returned when a write operation is attempted on a read-only transaction.
var ErrReadOnly = errors.New("read-only transaction")
