package provider_local

import "strings"

// ProviderID is the default ID for the local provider.
const ProviderID = "local"

// StorageVolumeID returns the storage volume id for a provider account.
func StorageVolumeID(providerID, accountID string) string {
	return strings.Join([]string{
		"p",
		providerID,
		accountID,
	}, "/")
}

// BlockStoreBucketID returns the bucket id for a block store.
func BlockStoreBucketID(providerID, accountID, blockStoreID string) string {
	return strings.Join([]string{
		"p",
		providerID,
		accountID,
		"blk",
		blockStoreID,
	}, "/")
}

// BlockStoreLocalID returns the local block store id for a block store.
func BlockStoreLocalID(providerID, accountID, blockStoreID string) string {
	// use the same id format as the bucket id
	return BlockStoreBucketID(providerID, accountID, blockStoreID)
}

// SobjectObjectStoreID returns the object store id for shared objects.
func SobjectObjectStoreID(providerID, accountID string) string {
	return strings.Join([]string{
		"p",
		providerID,
		accountID,
		"so",
	}, "/")
}

// SobjectBlockStoreID returns the block store id for a shared object.
// Block stores backing a shared object share the shared object's ULID
// verbatim; no prefix is added.
func SobjectBlockStoreID(sobjectID string) string {
	return sobjectID
}

// SobjectObjectStoreHostStateKey returns the object store key for a shared object with the given id.
func SobjectObjectStoreHostStateKey(sharedObjectID string) []byte {
	return []byte(strings.Join([]string{
		"so",
		sharedObjectID,
		"host",
	}, "/"))
}

// SobjectObjectStoreLocalStateKey returns the object store key for a shared object with the given id.
func SobjectObjectStoreLocalStateKey(sharedObjectID string) []byte {
	return []byte(strings.Join([]string{
		"so",
		sharedObjectID,
		ProviderID,
		"state",
	}, "/"))
}

// SobjectObjectStoreLocalOpResultKey returns the object store key for a shared object op result.
func SobjectObjectStoreLocalOpResultKey(sharedObjectID, opLocalID string) []byte {
	return []byte(strings.Join([]string{
		"so",
		sharedObjectID,
		ProviderID,
		"res",
		opLocalID,
	}, "/"))
}

// SessionObjectStoreID returns the object store id for sessions.
func SessionObjectStoreID(providerID, accountID string) string {
	return strings.Join([]string{
		"p",
		providerID,
		accountID,
		"sess",
	}, "/")
}

// SessionObjectStorePrivKey returns the object store key for the priv key for a session with the given id.
func SessionObjectStorePrivKey(sessionID string) []byte {
	return []byte(strings.Join([]string{
		sessionID,
		"pk",
	}, "/"))
}

// SessionObjectStoreStateKey returns the object store key for the state for a session with the given id.
func SessionObjectStoreStateKey(sessionID string) []byte {
	return []byte(strings.Join([]string{
		sessionID,
		"state",
	}, "/"))
}

// SobjectObjectStoreListKey returns the object store key for the shared object list.
func SobjectObjectStoreListKey() []byte {
	return []byte("so-list")
}

// SobjectBindingKey returns the object store key for a shared object binding purpose.
func SobjectBindingKey(purpose string) []byte {
	return []byte(strings.Join([]string{
		"so-binding",
		purpose,
	}, "/"))
}
