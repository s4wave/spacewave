//go:build !js

package storebench

// StoreConfig locates the World in a stopped server store.
type StoreConfig struct {
	// Dir is the storage root directory that holds the volume files.
	Dir string
	// PeerVolumeID is the storage volume whose peer runs the local provider.
	PeerVolumeID string
	// AccountID is the local provider account that holds the World.
	AccountID string
	// SharedObjectID is the World's SharedObject in the account.
	SharedObjectID string
	// BodyType is the body type of the SharedObject that Seed creates.
	BodyType string
	// EngineID is the World engine ID.
	EngineID string
	// CacheBytes is the memory budget of each s4db volume. Zero keeps the
	// native storage's, which a server runs with.
	CacheBytes int64
	// BlockCacheBytes is the decoded block cache budget. Zero keeps the
	// default.
	BlockCacheBytes int64
}
