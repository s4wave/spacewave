//go:build !linux && !js

package checkoutroot

import (
	"context"
	"errors"
	"os"
	"time"
)

// openAnonymousStage selects the named stage on hosts without O_TMPFILE.
func openAnonymousStage(*os.Root) (*os.File, error) {
	return nil, errors.ErrUnsupported
}

// publishAnonymousStage reports that these hosts cannot publish anonymous inodes.
func publishAnonymousStage(context.Context, *os.Root, *os.File, string, time.Time) error {
	return errors.ErrUnsupported
}
