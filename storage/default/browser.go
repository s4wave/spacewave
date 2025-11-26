//go:build js

package storage_default

import (
	storage "github.com/aperturerobotics/bldr/storage/browser"
)

// BuildStorage is the default storage provider.
var BuildStorage = storage.BuildStorage
