package provider_local

// LinkedCloudKey returns the ObjectStore key for the linked-cloud account ID.
func LinkedCloudKey(sessionID string) []byte {
	return []byte(sessionID + "/linked-cloud")
}
