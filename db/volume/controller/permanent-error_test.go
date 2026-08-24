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

// TestExecuteStopsAfterConsecutiveTransientFailures proves the consecutive
// construction-failure cap converts an endlessly retried transient failure
// (for example "opfs GetRoot: UnknownError") into a terminal error instead of
// looping forever under the loader's unbounded backoff.
func TestExecuteStopsAfterConsecutiveTransientFailures(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	transient := errors.New("opfs GetRoot: UnknownError")
	attempts := 0

	ctrl := NewController(le, &Config{}, nil, nil,
		func(ctx context.Context, le *logrus.Entry) (volume.Volume, error) {
			attempts++
			return nil, transient
		})

	for i := range maxConstructionAttempts - 1 {
		if err := ctrl.Execute(context.Background()); !errors.Is(err, transient) {
			t.Fatalf("attempt %d: Execute returned %v, want the transient error so it retries", i+1, err)
		}
	}
	if err := ctrl.Execute(context.Background()); err != nil {
		t.Fatalf("Execute returned %v after %d attempts, want nil so controllerbus stops restarting", err, maxConstructionAttempts)
	}
	if attempts != maxConstructionAttempts {
		t.Fatalf("constructor ran %d times, want %d", attempts, maxConstructionAttempts)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	vol, err := ctrl.GetVolume(ctx)
	if vol != nil {
		t.Fatalf("GetVolume returned a volume, want nil")
	}
	if !errors.Is(err, transient) {
		t.Fatalf("GetVolume returned %v, want it to wrap %v", err, transient)
	}
	if !volume.IsPermanent(err) {
		t.Fatalf("GetVolume error %v is not classified permanent", err)
	}
}
