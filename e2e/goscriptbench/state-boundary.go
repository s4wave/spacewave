//go:build !js

package goscriptbench

import (
	"slices"

	"github.com/pkg/errors"
)

// StateBoundary declares state retained and recreated between samples.
type StateBoundary struct {
	// retained names state preserved between samples
	Retained []string
	// recreated names state replaced between samples
	Recreated []string
}

// Validate checks that every state declaration is present and unambiguous.
func (s StateBoundary) Validate() error {
	if len(s.Retained) == 0 || len(s.Recreated) == 0 {
		return errors.New("retained and recreated state declarations are required")
	}
	for idx, name := range s.Retained {
		if name == "" {
			return errors.New("retained state contains an empty declaration")
		}
		if slices.Contains(s.Retained[:idx], name) {
			return errors.Errorf("retained state declaration %q is duplicated", name)
		}
	}
	for idx, name := range s.Recreated {
		if name == "" {
			return errors.New("recreated state contains an empty declaration")
		}
		if slices.Contains(s.Recreated[:idx], name) {
			return errors.Errorf("recreated state declaration %q is duplicated", name)
		}
		if slices.Contains(s.Retained, name) {
			return errors.Errorf("state declaration %q is both retained and recreated", name)
		}
	}
	return nil
}
