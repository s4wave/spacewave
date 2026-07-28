package plugin_space

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestWarnOnErrorUnlessCanceled(t *testing.T) {
	tests := []struct {
		name    string
		ctx     func() context.Context
		err     error
		wantLog bool
	}{
		{
			name: "operation error",
			ctx: func() context.Context {
				return context.Background()
			},
			err:     errors.New("read failed"),
			wantLog: true,
		},
		{
			name: "canceled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			err: errors.New("read failed"),
		},
		{
			name: "wrapped cancellation",
			ctx: func() context.Context {
				return context.Background()
			},
			err: errors.Join(errors.New("read failed"), context.Canceled),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := logrus.New()
			logger.SetOutput(&output)

			warnOnErrorUnlessCanceled(tt.ctx(), logrus.NewEntry(logger), tt.err, "operation failed")

			if got := output.Len() != 0; got != tt.wantLog {
				t.Fatalf("warning logged = %t, want %t; output = %q", got, tt.wantLog, output.String())
			}
		})
	}
}
