package resource_root

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	resource_cdn "github.com/s4wave/spacewave/core/resource/cdn"
	"github.com/s4wave/spacewave/core/session"
	"github.com/sirupsen/logrus"
)

func TestRootServerCloseBeforeStateAtomUse(t *testing.T) {
	var builds atomic.Int32
	server := newCloseTestServer(t)
	server.stateAtomStoreIndexBuilder = func(context.Context) (*session.StateAtomStoreIndex, func(), error) {
		builds.Add(1)
		return session.NewStateAtomStoreIndex(nil), func() {}, nil
	}

	server.Close()
	if builds.Load() != 0 {
		t.Fatalf("state atom store built after close: %d builds", builds.Load())
	}
	if _, err := server.getStateAtomStoreIndex(context.Background()); err == nil {
		t.Fatal("state atom store use succeeded after close")
	}
}

func TestRootServerCloseReleasesStateAtomAndCDN(t *testing.T) {
	var releases atomic.Int32
	server := newCloseTestServer(t)
	server.stateAtomStoreIndexBuilder = func(context.Context) (*session.StateAtomStoreIndex, func(), error) {
		return session.NewStateAtomStoreIndex(nil), func() {
			releases.Add(1)
		}, nil
	}
	if _, err := server.getStateAtomStoreIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := server.cdnRegistry.Lookup(""); err != nil {
		t.Fatalf("create CDN instance: %v", err)
	}

	server.Close()
	server.Close()

	if releases.Load() != 1 {
		t.Fatalf("state atom reference releases = %d, want 1", releases.Load())
	}
	if server.stateAtomStoreIndex != nil || server.releaseStateAtomStoreIndex != nil {
		t.Fatal("state atom handle remained cached after close")
	}
	if _, err := server.cdnRegistry.Lookup(""); !errors.Is(err, resource_cdn.ErrRegistryClosed) {
		t.Fatalf("CDN lookup after close error = %v, want %v", err, resource_cdn.ErrRegistryClosed)
	}
}

func TestRootServerCloseWaitsForConcurrentStateAtomUse(t *testing.T) {
	started := make(chan struct{})
	allow := make(chan struct{})
	var releases atomic.Int32
	server := newCloseTestServer(t)
	server.stateAtomStoreIndexBuilder = func(context.Context) (*session.StateAtomStoreIndex, func(), error) {
		close(started)
		<-allow
		return session.NewStateAtomStoreIndex(nil), func() {
			releases.Add(1)
		}, nil
	}

	useDone := make(chan error, 1)
	go func() {
		_, err := server.getStateAtomStoreIndex(context.Background())
		useDone <- err
	}()
	<-started

	closeDone := make(chan struct{})
	go func() {
		server.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("root close returned while state atom acquisition was blocked")
	case <-time.After(25 * time.Millisecond):
	}

	close(allow)
	if err := <-useDone; err != nil {
		t.Fatalf("state atom use: %v", err)
	}
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("root close did not complete after state atom acquisition")
	}
	if releases.Load() != 1 {
		t.Fatalf("state atom reference releases = %d, want 1", releases.Load())
	}
}

func newCloseTestServer(t *testing.T) *CoreRootServer {
	t.Helper()
	le := logrus.New().WithField("test", t.Name())
	return &CoreRootServer{
		le:          le,
		cdnRegistry: resource_cdn.NewRegistry(le, nil),
	}
}
