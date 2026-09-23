package sobject

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/net/peer"
)

// TestLeaveMissingSigner rejects consent that cannot identify its signing peer.
func TestLeaveMissingSigner(t *testing.T) {
	for _, signature := range []*peer.Signature{nil, {}} {
		request := &SOLeaveRequest{
			SharedObjectId: mockSharedObjectID,
			ConfigHash:     bytes.Repeat([]byte{1}, 32),
			Signatures:     []*peer.Signature{signature},
		}
		if _, err := request.Verify(); err == nil {
			t.Fatal("leave accepted a missing signer")
		}
	}
}
