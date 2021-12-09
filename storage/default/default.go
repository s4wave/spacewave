//go:build !js
// +build !js

package storage_default

import (
	storage "github.com/aperturerobotics/bldr/storage/desktop"
)

// BuildStorage is the default storage provider.
var BuildStorage = storage.BuildStorage
