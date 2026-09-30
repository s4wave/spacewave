package plugin_host

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/core"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestMissingPluginFilesFailsWithoutWaitingForCaller protects the worker startup
// path: exact manifest failure must settle even though no filesystem arrives.
func TestMissingPluginFilesFailsWithoutWaitingForCaller(t *testing.T) {
	// Bound the test with a three second timeout.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Start a bus and build the tracker for a missing artifact.
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	root, err := hash.Sum(hash.RecommendedHashType, []byte("missing"))
	if err != nil {
		t.Fatal(err)
	}
	server := &PluginHostServer{b: b, pluginID: "caller"}
	run, tracker := server.newPluginHostServerFsTracker(plugin.PluginArtifactID("colors", root.MarshalString()))

	// The execution must fail without waiting for a caller.
	if err := run(ctx); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing plugin files: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatal("missing artifact waited for the caller timeout")
	}

	// The retained failure must stay available to later waiters.
	if _, err := tracker.resultPromiseCtr.Await(ctx); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("retained failure: %v", err)
	}
}

// TestPluginFilesCanceledWithExecution joins a blocked filesystem lookup when
// the load owner reports failure, including its cause and release callback.
func TestPluginFilesCanceledWithExecution(t *testing.T) {
	// Bound the test with a three second timeout.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	b, _, err := core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}

	// Start a blocked filesystem lookup tied to a cancellable lifetime.
	tracker := &pluginHostServerFsTracker{s: &PluginHostServer{b: b}}
	lifetime, fail := context.WithCancelCause(ctx)
	defer fail(context.Canceled)
	failure := errors.New("module startup failed")
	done := make(chan error, 1)
	released := make(chan struct{})
	go func() {
		_, release, err := tracker.accessFiles(lifetime, "missing-files")(ctx, func() { close(released) })
		if release != nil {
			release()
		}
		done <- err
	}()

	// The lookup must fail with the lifetime's cause.
	fail(failure)
	select {
	case err := <-done:
		if !errors.Is(err, failure) {
			t.Fatalf("file lookup lost load failure: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("file lookup did not follow the execution lifetime")
	}

	// The release callback must run when the lifetime fails.
	select {
	case <-released:
	case <-ctx.Done():
		t.Fatal("cursor release was not reported")
	}
}
