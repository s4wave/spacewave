//go:build !unix

package forge_lib_skiffos

import (
	"context"

	"github.com/pkg/errors"
)

// runMake refuses the build: SkiffOS builds on a Unix Worker host.
func runMake(ctx context.Context, dir, logPath string, env []string, targets ...string) error {
	return errors.New("SkiffOS builds need a Unix Worker host")
}
