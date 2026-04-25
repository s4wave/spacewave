package bstore

import (
	"context"

	provider "github.com/s4wave/spacewave/core/provider"
)

// BlockStoreProvider implements ProviderFeature_BLOCK_STORE.
type BlockStoreProvider interface {
	provider.ProviderAccountFeature

	// CreateBlockStore creates a new block store with the given details.
	CreateBlockStore(ctx context.Context, id string) (*BlockStoreRef, error)

	// MountBlockStore attempts to mount a BlockStore returning the object handle and a release function.
	//
	// note: use the MountBlockStore directive to call this.
	// usually called by the provider controller
	MountBlockStore(ctx context.Context, ref *BlockStoreRef, released func()) (BlockStore, func(), error)
}

// GetBlockStoreProviderAccountFeature returns the BlockStoreProvider for a ProviderAccount.
func GetBlockStoreProviderAccountFeature(ctx context.Context, provAcc provider.ProviderAccount) (BlockStoreProvider, error) {
	return provider.GetProviderAccountFeature[BlockStoreProvider](
		ctx,
		provAcc,
		provider.ProviderFeature_ProviderFeature_BLOCK_STORE,
	)
}
