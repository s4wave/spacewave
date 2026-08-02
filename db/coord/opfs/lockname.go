package opfs

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/s4wave/spacewave/db/coord"
)

// writeLockName derives the Web Lock name guarding a scope under one prefix.
func writeLockName(prefix string, scope coord.Scope) string {
	digest := sha256.Sum256([]byte(
		scope.VolumeID + "\x00" + scope.ObjectStoreID + "\x00" + scope.Key,
	))
	return prefix + "/coord/write/" + hex.EncodeToString(digest[:])
}
