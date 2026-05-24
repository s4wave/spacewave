package provider_spacewave

import (
	"time"

	"github.com/s4wave/spacewave/core/provider/spacewave/entitykeystore"
)

// EntityKeyStore tracks unlocked entity private keys in memory.
type EntityKeyStore = entitykeystore.EntityKeyStore

// EntityKeyStoreRef is a retention ref for unlocked entity keys.
type EntityKeyStoreRef = entitykeystore.EntityKeyStoreRef

// DefaultEntityKeyStoreGrace is the default post-release key retention window.
const DefaultEntityKeyStoreGrace = entitykeystore.DefaultEntityKeyStoreGrace

// NewEntityKeyStore creates a new EntityKeyStore.
func NewEntityKeyStore() *EntityKeyStore {
	return entitykeystore.NewEntityKeyStore()
}

// NewEntityKeyStoreWithGrace creates a new EntityKeyStore with a grace timer.
func NewEntityKeyStoreWithGrace(grace time.Duration) *EntityKeyStore {
	return entitykeystore.NewEntityKeyStoreWithGrace(grace)
}
