//go:build !js

package compose

import (
	"context"

	"github.com/sirupsen/logrus"
)

// NativeAction handles a project's no-argument native launch before a writable
// distribution bus is built. It owns its own client or runtime lifetime and
// returns when that operation finishes. Command dispatch still uses Commands.
type NativeAction func(context.Context, *logrus.Entry) error
