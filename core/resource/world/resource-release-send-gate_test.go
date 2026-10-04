//go:build !js

package resource_world_test

import (
	"sync/atomic"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
)

// resourceReleaseSendGate holds one exact grant's release before wire delivery.
type resourceReleaseSendGate struct {
	// Stream preserves the actual ResourceClient transport and context.
	srpc.Stream
	// grantID selects the exact snapshot whose notification is withheld.
	grantID atomic.Uint32
	// entered reports that the server queued the selected release.
	entered chan struct{}
	// resume permits the normal transport to deliver the selected release.
	resume chan struct{}
}

// MsgSend gates only the selected release and forwards every other response.
func (s *resourceReleaseSendGate) MsgSend(message srpc.Message) error {
	// Delay the exact grant's release without blocking acquisition result delivery.
	response, ok := message.(*resource.ResourceClientResponse)
	if ok && response.GetResourceReleased() != nil && response.GetResourceReleased().GetResourceId() == s.grantID.Load() {
		close(s.entered)
		select {
		case <-s.resume:
		case <-s.Context().Done():
			return s.Context().Err()
		}
	}

	// Preserve the ResourceClient protocol outside the selected release barrier.
	return s.Stream.MsgSend(message)
}

// release permits delivery and is safe to repeat during fixture teardown.
func (s *resourceReleaseSendGate) release() {
	select {
	case <-s.resume:
	default:
		close(s.resume)
	}
}
