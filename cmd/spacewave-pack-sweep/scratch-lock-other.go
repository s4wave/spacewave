//go:build !darwin && !linux

package main

import (
	"os"

	"github.com/pkg/errors"
)

// lockScratch refuses on a platform with no lock the command uses, since
// two steps of a cycle must not interleave.
func lockScratch(string) (*os.File, error) {
	return nil, errors.New("the scratch lock needs Linux or macOS")
}
