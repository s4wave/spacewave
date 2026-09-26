package plugin_space_runtime

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	plugin_host_mock "github.com/s4wave/spacewave/bldr/plugin/host/mock"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	"github.com/s4wave/spacewave/testbed"
)

func TestStartControllerWithConfigSharesOneRuntime(t *testing.T) {
	tb := newTestbed(t)
	conf := newTestConfig(tb, "space-a")
	first, firstRef, err := StartControllerWithConfig(t.Context(), tb.Bus, conf)
	if err != nil {
		t.Fatal(err)
	}
	second, secondRef, err := StartControllerWithConfig(t.Context(), tb.Bus, conf.CloneVT())
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equal configs started two runtimes")
	}
	gen := waitGeneration(t, first, nil)

	other, otherRef, err := StartControllerWithConfig(t.Context(), tb.Bus, newTestConfig(tb, "space-b"))
	if err != nil {
		t.Fatal(err)
	}
	otherRef.Release()
	if other == first {
		t.Fatal("different Spaces shared one runtime")
	}

	// The runtime outlives every reference but the last.
	firstRef.Release()
	select {
	case <-gen.Done():
		t.Fatal("releasing one of two references stopped the runtime")
	case <-time.After(50 * time.Millisecond):
	}
	secondRef.Release()
	select {
	case <-gen.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("releasing the last reference did not stop the runtime")
	}
}

func TestControllerRetriesAfterHostWatchFailure(t *testing.T) {
	tb := newTestbed(t)
	rt, ref, err := StartControllerWithConfig(t.Context(), tb.Bus, newTestConfig(tb, "space-a"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
	first := waitGeneration(t, rt, nil)

	releaseHostError, err := tb.Bus.AddController(
		t.Context(),
		plugin_host_mock.NewLookupErrorController(errors.New("test plugin host watch error")),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	err = waitFailure(t, rt)
	if err.Error() != "watch daemon plugin hosts: test plugin host watch error" {
		t.Fatalf("runtime failure = %q", err.Error())
	}
	select {
	case <-first.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("failed generation was not released")
	}

	// The runtime keeps its identity and starts a new generation once the
	// watch recovers.
	releaseHostError()
	waitGeneration(t, rt, first)
}

func TestReserveServicePrefix(t *testing.T) {
	c := &Controller{prefixes: make(map[string]struct{})}
	release, err := c.ReserveServicePrefix("attached/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReserveServicePrefix("attached/"); err == nil {
		t.Fatal("a bound prefix was reserved twice")
	}
	if _, err := c.ReserveServicePrefix("other/"); err != nil {
		t.Fatalf("an unbound prefix was refused: %v", err)
	}

	release()
	if _, err := c.ReserveServicePrefix("attached/"); err != nil {
		t.Fatalf("a released prefix was refused: %v", err)
	}
}

// newTestbed starts a testbed that stops when the test ends.
func newTestbed(t *testing.T) *testbed.Testbed {
	t.Helper()
	tb, err := testbed.Default(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	return tb
}

// newTestConfig returns a runtime config for spaceID on the testbed engine.
func newTestConfig(tb *testbed.Testbed, spaceID string) *Config {
	return &Config{Space: &plugin_space.Config{
		SpaceId:       spaceID,
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	}}
}

// waitGeneration waits for rt to run a generation other than prev.
func waitGeneration(t *testing.T, rt *Controller, prev *Generation) *Generation {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		gen, waitCh, _ := rt.GetGeneration()
		if gen != nil && gen != prev {
			return gen
		}
		select {
		case <-waitCh:
		case <-timeout:
			t.Fatal("timed out waiting for a runtime generation")
		}
	}
}

// waitFailure waits for rt to publish a failure and returns it.
func waitFailure(t *testing.T, rt *Controller) error {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		gen, waitCh, err := rt.GetGeneration()
		if gen == nil && err != nil {
			return err
		}
		select {
		case <-waitCh:
		case <-timeout:
			t.Fatal("timed out waiting for a runtime failure")
		}
	}
}
