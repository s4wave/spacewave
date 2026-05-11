package plugin_host_configset

import (
	"testing"

	"github.com/pkg/errors"
)

func TestIsWebRuntimeClientClosed(t *testing.T) {
	if !isWebRuntimeClientClosed(errors.New("WebRuntimeClientInstance closed: plugin/spacewave-core")) {
		t.Fatal("expected WebRuntimeClientInstance closed error to be classified")
	}
	if isWebRuntimeClientClosed(errors.New("WebRuntimeClientInstance is closed")) {
		t.Fatal("expected generic closed error to keep existing error handling")
	}
	if isWebRuntimeClientClosed(nil) {
		t.Fatal("expected nil error to be unclassified")
	}
}
