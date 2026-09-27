package sobject_world_engine

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// operationPerson resolves the signer's entity from one accepted config snapshot.
// A participant without an entity uses its signing device as the person.
func operationPerson(ctx context.Context, snapshot sobject.SharedObjectStateSnapshot, device peer.ID) (string, error) {
	participant, err := snapshot.GetParticipantConfigForPeer(ctx, device.String())
	if err != nil {
		return "", err
	}
	if person := participant.GetEntityId(); person != "" {
		return person, nil
	}
	return device.String(), nil
}
