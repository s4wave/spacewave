package web_pkg_rpc_server

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/keyed"
	"github.com/sirupsen/logrus"
)

func TestCloseJoinsPostExecuteTrackerWork(t *testing.T) {
	ctrl, err := NewController(logrus.NewEntry(logrus.New()), nil, NewConfig("", []string{"pkg"}))
	if err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	stopped := make(chan struct{})
	ctrl.webPkgs = keyed.NewKeyedRefCount(func(string) (keyed.Routine, *webPkgTracker) {
		return ctrl.routines.wrap(func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			<-release
			close(stopped)
			return nil
		}), &webPkgTracker{}
	})
	ctrl.webPkgs.SetContext(context.Background(), true)
	ctrl.webPkgs.AddKeyRef("post-execute")
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("web-package tracker did not start")
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
}
