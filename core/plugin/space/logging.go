package plugin_space

import (
	"context"
	"errors"

	"github.com/sirupsen/logrus"
)

func warnOnErrorUnlessCanceled(ctx context.Context, le *logrus.Entry, err error, message string) {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return
	}
	le.WithError(err).Warn(message)
}
