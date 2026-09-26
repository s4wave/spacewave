package plugin_space_runtime

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_mock "github.com/s4wave/spacewave/bldr/plugin/host/mock"
)

func TestHostWatchIdenticalRedeliveryKeepsGeneration(t *testing.T) {
	w, _, _ := newTestHostWatch()
	if err := w.deliver(nil, testHosts("a", "b")); err != nil {
		t.Fatalf("initial delivery returned %v", err)
	}
	if err := w.deliver(nil, testHosts("b", "a")); err != nil {
		t.Fatalf("identical redelivery ended the generation: %v", err)
	}
}

func TestHostWatchInitialErrorFailsStartup(t *testing.T) {
	w, hostReady, terminal := newTestHostWatch()
	sent := errors.New("resolver failed")
	if err := w.deliver([]error{sent}, nil); err != nil {
		t.Fatalf("error delivery returned %v", err)
	}
	select {
	case err := <-hostReady:
		if !errors.Is(err, sent) {
			t.Fatalf("startup error = %v, want %v", err, sent)
		}
	case <-time.After(time.Second):
		t.Fatal("startup did not fail with the watch error")
	}
	select {
	case err := <-terminal:
		t.Fatalf("initial error ended a generation: %v", err)
	default:
	}
}

func TestHostWatchLaterErrorEndsGeneration(t *testing.T) {
	w, _, terminal := newTestHostWatch()
	if err := w.deliver(nil, testHosts("a")); err != nil {
		t.Fatalf("initial delivery returned %v", err)
	}
	sent := errors.New("watch broke")
	if err := w.deliver([]error{sent}, nil); err != nil {
		t.Fatalf("error delivery returned %v", err)
	}
	select {
	case err := <-terminal:
		if !errors.Is(err, sent) {
			t.Fatalf("terminal error = %v, want %v", err, sent)
		}
		if err.Error() != "watch daemon plugin hosts: watch broke" {
			t.Fatalf("terminal error = %q, want the watch named", err.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("later watch error did not end the generation")
	}
}

func TestHostWatchEmptySetEndsGeneration(t *testing.T) {
	w, _, _ := newTestHostWatch()
	if err := w.deliver(nil, testHosts("a", "b")); err != nil {
		t.Fatalf("initial delivery returned %v", err)
	}
	if err := w.deliver(nil, nil); !errors.Is(err, errPluginHostSetChanged) {
		t.Fatalf("empty delivery = %v, want %v", err, errPluginHostSetChanged)
	}
}

func TestHostWatchMembershipChangeEndsGeneration(t *testing.T) {
	w, _, _ := newTestHostWatch()
	if err := w.deliver(nil, testHosts("a", "b")); err != nil {
		t.Fatalf("initial delivery returned %v", err)
	}
	if err := w.deliver(nil, testHosts("a")); !errors.Is(err, errPluginHostSetChanged) {
		t.Fatalf("membership loss = %v, want %v", err, errPluginHostSetChanged)
	}
}

func TestHostWatchDuplicateGrowthEndsGeneration(t *testing.T) {
	w, _, _ := newTestHostWatch()
	if err := w.deliver(nil, testHosts("a")); err != nil {
		t.Fatalf("initial delivery returned %v", err)
	}
	if err := w.deliver(nil, testHosts("a", "a")); !errors.Is(err, errPluginHostSetChanged) {
		t.Fatalf("duplicate growth = %v, want %v", err, errPluginHostSetChanged)
	}
}

// newTestHostWatch returns a host watch with its startup and terminal channels.
func newTestHostWatch() (*hostWatch, <-chan error, <-chan error) {
	hostReady := make(chan error, 1)
	terminal := make(chan error, 8)
	w := &hostWatch{
		hostReady:      hostReady,
		reportTerminal: func(err error) { terminal <- err },
	}
	return w, hostReady, terminal
}

// testHosts returns one test plugin host per platform ID.
func testHosts(platformIDs ...string) []plugin_host.PluginHost {
	hosts := make([]plugin_host.PluginHost, 0, len(platformIDs))
	for _, id := range platformIDs {
		hosts = append(hosts, plugin_host_mock.NewHost(id))
	}
	return hosts
}
