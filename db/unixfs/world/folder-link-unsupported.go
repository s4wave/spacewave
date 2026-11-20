//go:build !darwin && !linux

package unixfs_world

import "github.com/pkg/errors"

// twoWaySupported reports that this platform lacks the renames and the trash
// a two-way link needs.
const twoWaySupported = false

// errExchangeUnsupported marks a file system without an atomic exchange.
var errExchangeUnsupported = errors.New("file system cannot exchange files atomically")

// errTwoWayUnsupported is returned by the renames and the trash here.
var errTwoWayUnsupported = errors.New("two-way linked folders are not supported on this platform")

// renameNoReplace is unsupported here.
func renameNoReplace(string, string) error {
	return errTwoWayUnsupported
}

// exchange is unsupported here.
func exchange(string, string) error {
	return errExchangeUnsupported
}

// moveToTrash is unsupported here.
func moveToTrash(string) (string, func() error, error) {
	return "", nil, errTwoWayUnsupported
}
