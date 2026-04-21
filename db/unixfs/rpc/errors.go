package unixfs_rpc

import "errors"

var (
	// ErrClientIDEmpty is returned if the client id was empty.
	ErrClientIDEmpty = errors.New("client id cannot be zero")
	// ErrHandleIDEmpty is returned if the handle id was empty.
	ErrHandleIDEmpty = errors.New("handle id cannot be zero")
)
