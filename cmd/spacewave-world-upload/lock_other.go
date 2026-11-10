//go:build !unix

package main

import (
	"os"

	"github.com/pkg/errors"
)

// lockFile refuses to run where flock is not available.
func lockFile(*os.File) error {
	return errors.New("flock is not supported on this platform")
}
