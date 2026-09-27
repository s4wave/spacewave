package electron

import "github.com/pkg/errors"

// errDesktopClosed is returned to an open request when its shell exits before acknowledgement.
var errDesktopClosed = errors.New("electron desktop closed before open acknowledgement")
