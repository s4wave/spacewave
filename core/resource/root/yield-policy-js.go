//go:build js

package resource_root

import (
	"context"

	"github.com/pkg/errors"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// errYieldPolicyUnsupported is returned by the js stubs that do not
// support the desktop resource listener yield broker.
var errYieldPolicyUnsupported = errors.New("yield policy not supported on js")

// WatchListenerYieldPrompts is unsupported on js.
func (s *CoreRootServer) WatchListenerYieldPrompts(
	_ *s4wave_root.WatchListenerYieldPromptsRequest,
	_ s4wave_root.SRPCRootResourceService_WatchListenerYieldPromptsStream,
) error {
	return errYieldPolicyUnsupported
}

// RespondToListenerYieldPrompt is unsupported on js.
func (s *CoreRootServer) RespondToListenerYieldPrompt(
	_ context.Context,
	_ *s4wave_root.RespondToListenerYieldPromptRequest,
) (*s4wave_root.RespondToListenerYieldPromptResponse, error) {
	return nil, errYieldPolicyUnsupported
}

// WatchRuntimeHandoff is unsupported on js.
func (s *CoreRootServer) WatchRuntimeHandoff(
	_ *s4wave_root.WatchRuntimeHandoffRequest,
	_ s4wave_root.SRPCRootResourceService_WatchRuntimeHandoffStream,
) error {
	return errYieldPolicyUnsupported
}

// ReclaimRuntime is unsupported on js.
func (s *CoreRootServer) ReclaimRuntime(
	_ context.Context,
	_ *s4wave_root.ReclaimRuntimeRequest,
) (*s4wave_root.ReclaimRuntimeResponse, error) {
	return nil, errYieldPolicyUnsupported
}
