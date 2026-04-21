package pubmessage

import "errors"

// ErrInvalidChannelID is returned for an invalid inner channel ID.
var ErrInvalidChannelID = errors.New("invalid or empty channel id on pubmessage")
