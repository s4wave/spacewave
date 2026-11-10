//go:build !linux

package main

import (
	"os"

	"github.com/pkg/errors"
)

func lockExclusive(*os.File, string) (func(), error) {
	return nil, errors.New("exclusive OFD locks need Linux")
}

func punchRanges(*os.File, header, []pageRange) error {
	return errors.New("punch needs Linux")
}

func allocated(os.FileInfo) int64 { return -1 }
