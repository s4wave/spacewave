package provider_spacewave

import "strings"

// ProviderIDPrefix is the prefix for spacewave provider resource IDs.
const ProviderIDPrefix = "p/spacewave/"

// ProviderResourceID returns the resource ID for a spacewave provider resource.
func ProviderResourceID(accountID, resourceType, resourceID string) string {
	return strings.Join([]string{
		ProviderIDPrefix + accountID,
		resourceType,
		resourceID,
	}, "/")
}

// BlockStoreID returns the block store ID for a spacewave provider block store.
func BlockStoreID(accountID, bstoreID string) string {
	return ProviderResourceID(accountID, "blk", bstoreID)
}

// SharedObjectID returns the shared object ID for a spacewave provider shared object.
func SharedObjectID(accountID, soID string) string {
	return ProviderResourceID(accountID, "so", soID)
}

// SessionID returns the session ID for a spacewave provider session.
func SessionID(accountID, sessionID string) string {
	return ProviderResourceID(accountID, "sess", sessionID)
}

// StorageVolumeID returns the storage volume id for a provider account.
func StorageVolumeID(accountID string) string {
	return strings.Join([]string{
		ProviderIDPrefix + accountID,
		"vol",
	}, "/")
}

// BlockStoreObjectStoreID returns the object store ID for a block store's metadata.
func BlockStoreObjectStoreID(accountID, bstoreID string) string {
	return ProviderIDPrefix + accountID + "/bstore/" + bstoreID + "/meta"
}

// BlockStoreBucketID returns the bucket ID for a block store's upper cache.
func BlockStoreBucketID(accountID, bstoreID string) string {
	return strings.Join([]string{
		ProviderIDPrefix + accountID,
		"bstore",
		bstoreID,
		"cache",
	}, "/")
}

// SessionObjectStoreID returns the object store ID for sessions.
func SessionObjectStoreID(accountID string) string {
	return strings.Join([]string{
		ProviderIDPrefix + accountID,
		"sess",
	}, "/")
}

// AccountStateCacheID returns the object store ID for the account state cache.
func AccountStateCacheID(accountID string) string {
	return ProviderIDPrefix + accountID + "/account-state"
}
