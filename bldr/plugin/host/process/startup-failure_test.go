//go:build !js && !windows

package plugin_host_process

import (
	"strings"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
)

// TestStartupProtocolFailure preserves stderr evidence across a real child exit.
func TestStartupProtocolFailure(t *testing.T) {
	// A native child can fail before connecting its RPC, as incompatible plugins do.
	host := newTestProcessHost(t)
	dist := newTestDistHandle(t, map[string][]byte{
		"entrypoint": []byte("#!/bin/sh\necho 'complete initial capability registration: proto: wrong wireType = 2 for field ResourceId' >&2\nexit 1\n"),
	})
	t.Cleanup(dist.Release)
	err := host.ExecutePlugin(t.Context(), "sample", "", "", "sample-build", "entrypoint",
		dist, dist, srpc.NewMux(), func(srpc.Client) error { return nil })
	var protocolErr *plugin_host.StartupProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("process failure = %v, want startup protocol error", err)
	}
	for _, expected := range []string{"host build sha256:", "plugin build sample manifest=sample-build sha256:", "wrong wireType"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("diagnostic %q omits %q", err, expected)
		}
	}
}

// TestStartupFailureLeavesOtherExitsRetriable keeps ordinary failures distinct.
func TestStartupFailureLeavesOtherExitsRetriable(t *testing.T) {
	exitErr := errors.New("exit status 1")
	for _, line := range []string{
		"complete initial capability registration: connection reset",
		"application operation: proto: wrong wireType = 2",
	} {
		if got := startupFailure("sample", "root", "unused", []string{line}, exitErr); got != exitErr {
			t.Fatalf("failure %q became terminal: %v", line, got)
		}
	}
}
