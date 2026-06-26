package volume_controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// TestExecuteStopsOnPermanentError proves a permanent construction failure ends
// Execute without an error (so controllerbus does not restart the controller)
// and that GetVolume returns the failure instead of blocking forever.
func TestExecuteStopsOnPermanentError(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	cause := errors.New("opfs GetRoot: SecurityError")
	wantErr := volume.Permanent(cause)

	ctrl := NewController(le, &Config{}, nil, nil,
		func(ctx context.Context, le *logrus.Entry) (volume.Volume, error) {
			return nil, wantErr
		})

	if err := ctrl.Execute(context.Background()); err != nil {
		t.Fatalf("Execute returned %v, want nil so controllerbus does not retry", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	vol, err := ctrl.GetVolume(ctx)
	if vol != nil {
		t.Fatalf("GetVolume returned a volume, want nil")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("GetVolume returned %v, want it to wrap %v", err, cause)
	}
	if !volume.IsPermanent(err) {
		t.Fatalf("GetVolume error %v is not classified permanent", err)
	}
}

// TestExecuteRetriesTransientError proves a non-permanent construction failure
// still propagates out of Execute so controllerbus restarts with backoff.
func TestExecuteRetriesTransientError(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	transient := errors.New("temporary failure")

	ctrl := NewController(le, &Config{}, nil, nil,
		func(ctx context.Context, le *logrus.Entry) (volume.Volume, error) {
			return nil, transient
		})

	if err := ctrl.Execute(context.Background()); !errors.Is(err, transient) {
		t.Fatalf("Execute returned %v, want the transient error so it retries", err)
	}
	if got := ctrl.getTerminal(); got != nil {
		t.Fatalf("getTerminal returned %v, want nil for a transient error", got)
	}
}
