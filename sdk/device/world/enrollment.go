package s4wave_device_world

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/s4wave/spacewave/net/peer"
)

// EnrolledDeviceObjectKey returns the World key of the Device for a peer: a
// 128-bit hash of its printable identity. SpaceLink approval reports it and
// device setup writes the Device there.
func EnrolledDeviceObjectKey(id peer.ID) string {
	sum := sha256.Sum256([]byte(id.String()))
	return "devices/" + hex.EncodeToString(sum[:16])
}
