//go:build js

package storage_default

import (
	storage "github.com/s4wave/spacewave/bldr/storage/browser"
)

// BuildStorage is the default storage provider.
var BuildStorage = storage.BuildStorage
