package bstore_world

import (
	"context"
	"strings"

	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

const (
	// BlockStoreStatePrefix is the prefix applied to local bstore IDs.
	BlockStoreStatePrefix = "bstore/"

	// BlockStoreStateTypeID is the type identifier for a BlockStoreState.
	//
	// The object contents is a BlockStoreStateInfo message.
	BlockStoreStateTypeID = "bstore/state"
)

// NewBlockStoreStateKey builds a key from a local bstore id.
func NewBlockStoreStateKey(providerID, providerAccID, bstoreID string) string {
	return strings.Join([]string{
		BlockStoreStatePrefix,
		providerID,
		"/",
		providerAccID,
		"/",
		bstoreID,
	}, "")
}

// LookupBlockStoreState looks up a local bstore object with the given object key.
// returns nil, nil, ErrObjectNotFound if not found.
func LookupBlockStoreState(ctx context.Context, w world.WorldState, objKey string) (*BlockStoreState, world.ObjectState, error) {
	return world.LookupObject[*BlockStoreState](ctx, w, objKey, NewBlockStoreStateBlock)
}

// ListBlockStoreStates returns all BlockStoreState objects in the world by object key.
// returns nil, nil if not found.
func ListBlockStoreStates(ctx context.Context, w world.WorldState) ([]string, error) {
	return world_types.ListObjectsWithType(ctx, w, BlockStoreStateTypeID)
}

// CollectBlockStoreStates collects all local bstore objects in the world.
func CollectBlockStoreStates(ctx context.Context, w world.WorldState) ([]*BlockStoreState, []string, error) {
	return world_types.ListCollectObjectsWithType[*BlockStoreState](ctx, w, BlockStoreStateTypeID, NewBlockStoreStateBlock)
}
