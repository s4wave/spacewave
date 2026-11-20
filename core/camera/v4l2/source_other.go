//go:build !linux

// Package v4l2 captures compressed frames from a Linux video device.
package v4l2

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/frametrack"
)

// NewSource reports that V4L2 capture requires Linux.
func NewSource(path string) (frametrack.Source, error) {
	return nil, errors.Errorf("camera device %q requires Linux V4L2", path)
}
