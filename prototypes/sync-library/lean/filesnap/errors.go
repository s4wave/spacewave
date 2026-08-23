package volume_filesnap

import "errors"

// errPathEmpty is returned when the snapshot path is empty.
var errPathEmpty = errors.New("path cannot be empty")
