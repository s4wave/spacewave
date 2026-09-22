package world_block

import "encoding/base64"

// retainedRootName isolates heads sharing one block bucket.
func (e *Engine) retainedRootName() string {
	return RetainedRootName(e.writeCoordScope.ObjectStoreID, e.writeCoordKeyPrefix)
}

// RetainedRootName identifies the durable World head within a block bucket.
func RetainedRootName(objectStoreID string, key []byte) string {
	return "world/" + base64.RawURLEncoding.EncodeToString([]byte(objectStoreID)) + "/" + base64.RawURLEncoding.EncodeToString(key)
}
