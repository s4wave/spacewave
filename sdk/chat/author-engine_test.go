package spacewave_chat

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// chatAuthorEngine supplies a test participant while retaining real World storage.
type chatAuthorEngine struct {
	world.Engine
	device peer.ID
	person string
}

// OperationAuthor returns the test participant's accepted author.
func (e *chatAuthorEngine) OperationAuthor(context.Context) (peer.ID, string, error) {
	return e.device, e.person, nil
}

// newChatResource constructs a local-only test participant.
func newChatResource(t *testing.T, ws world.WorldState, engine world.Engine, objectKey, device string) *ChatResource {
	t.Helper()
	return newChatResourceForPerson(t, ws, engine, objectKey, device, device)
}

// newChatResourceForPerson constructs a test participant with an accepted entity.
func newChatResourceForPerson(t *testing.T, ws world.WorldState, engine world.Engine, objectKey, device, person string) *ChatResource {
	// Attribute author fixture errors to the calling test.
	t.Helper()

	// Bind the test participant to a World engine with an accepted author.
	if engine != nil {
		engine = &chatAuthorEngine{Engine: engine, device: peer.ID(device), person: person}
	}

	// Construct the channel Resource through its production author binding.
	resource, err := NewChatResource(t.Context(), ws, engine, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	return resource
}
