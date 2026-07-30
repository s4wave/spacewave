//go:build !js

package bldr_project_controller

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/keyed"
	"github.com/sirupsen/logrus"
)

func TestCloseJoinsPostExecuteTrackerWork(t *testing.T) {
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, &Config{})
	if err := ctrl.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	stopped := make(chan struct{})
	ctrl.manifestBuilders = keyed.NewKeyedRefCount(func(string) (keyed.Routine, *manifestBuilderTracker) {
		return ctrl.routines.Wrap(func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			<-release
			close(stopped)
			return nil
		}), &manifestBuilderTracker{}
	})
	ctrl.manifestBuilders.SetContext(context.Background(), true)
	ctrl.manifestBuilders.AddKeyRef("post-execute")
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("manifest tracker did not start")
	}

	closeDone := make(chan struct{})
	go func() {
		_ = ctrl.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("Close returned before the tracker stopped")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after the tracker stopped")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("Close returned before the tracker stop signal")
	}
	if err := ctrl.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := ctrl.AddRemoteRef("after-close")
	if err != errControllerClosed {
		t.Fatalf("AddRemoteRef after Close: got %v, want %v", err, errControllerClosed)
	}
}
