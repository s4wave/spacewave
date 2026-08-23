package provider_spacewave

import (
	"crypto/sha256"
	"strconv"
	"strings"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

const (
	friendDmIDDomain       = "spacewave/friend-dm/v1"
	friendDmULIDTimestamp  = int64(1262304000000)
	friendDmCrockfordLower = "0123456789abcdefghjkmnpqrstvwxyz"
)

func encodeFriendDmTimestamp(timestamp int64) string {
	chars := make([]byte, 10)
	value := timestamp
	for index := len(chars) - 1; index >= 0; index-- {
		chars[index] = friendDmCrockfordLower[value&0x1f]
		value >>= 5
	}
	return string(chars)
}

func encodeFriendDmRandomPart(bytes []byte) string {
	chars := make([]byte, 16)
	for index := range chars {
		bitOffset := index * 5
		byteIndex := bitOffset / 8
		shift := bitOffset % 8
		bits := uint16(bytes[byteIndex]) << 8
		if byteIndex+1 < len(bytes) {
			bits |= uint16(bytes[byteIndex+1])
		}
		chars[index] = friendDmCrockfordLower[(bits>>uint(11-shift))&0x1f]
	}
	return string(chars)
}

func deriveFriendDmSharedObjectID(
	first *api.FriendDmAccount,
	second *api.FriendDmAccount,
) string {
	low, high := first, second
	if high.GetAccountId() < low.GetAccountId() {
		low, high = high, low
	}
	payload := strings.Join([]string{
		friendDmIDDomain,
		low.GetAccountId(),
		low.GetEntityUuid(),
		strconv.FormatUint(low.GetEpoch(), 10),
		high.GetAccountId(),
		high.GetEntityUuid(),
		strconv.FormatUint(high.GetEpoch(), 10),
	}, "\x00")
	digest := sha256.Sum256([]byte(payload))
	return encodeFriendDmTimestamp(friendDmULIDTimestamp) + encodeFriendDmRandomPart(digest[:10])
}
