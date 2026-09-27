package sobject_world_engine

import (
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// TestOperationPerson binds two signing devices to one accepted entity and
// falls back to the signing device for a local-only participant.
func TestOperationPerson(t *testing.T) {
	first, second, local := peer.ID("device-a"), peer.ID("device-b"), peer.ID("local-only")
	snapshot := &testSharedObjectSnapshot{participants: map[string]*sobject.SOParticipantConfig{
		first.String():  {PeerId: first.String(), EntityId: "person"},
		second.String(): {PeerId: second.String(), EntityId: "person"},
		local.String():  {PeerId: local.String()},
	}}
	for _, test := range []struct {
		device peer.ID
		person string
	}{
		{first, "person"},
		{second, "person"},
		{local, local.String()},
	} {
		person, err := operationPerson(t.Context(), snapshot, test.device)
		if err != nil || person != test.person {
			t.Fatalf("person for %s = %q, %v", test.device, person, err)
		}
	}
}
