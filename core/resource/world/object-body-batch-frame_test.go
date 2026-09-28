package resource_world

import (
	"math"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// starpcMaxMessageSize is the largest packet a starpc stream accepts.
const starpcMaxMessageSize = 10_000_000

// TestObjectBodiesBatchPageFitsStarpcFrame checks that a full body page, sent
// as one stream message, stays within the starpc packet limit.
func TestObjectBodiesBatchPageFitsStarpcFrame(t *testing.T) {
	page := &s4wave_world.GetObjectBodiesBatchResponse{
		Bodies: []*s4wave_world.ObjectBody{{
			ObjectKey: "object",
			Body:      make([]byte, world.ObjectBodiesBatchByteBudget),
			Exists:    true,
			Rev:       math.MaxUint64,
		}},
		WorldSeqno: math.MaxUint64,
	}
	data, err := page.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if size := srpc.NewCallDataPacket(data, false, false, nil).SizeVT(); size > starpcMaxMessageSize {
		t.Fatalf("page packet size = %d, exceeds starpc maximum %d", size, starpcMaxMessageSize)
	}
}
