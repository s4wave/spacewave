package packedmsg

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc64"
	"strings"
)

var (
	baseMagic   = []byte{0x4, 0x2, 0x0}
	secondMagic = [8]byte{0x4c, 0x47, 0x4c, 0x48, 0x4d, 0x0, 0x4, 0x2}
)

func xor(data []byte) []byte {
	out := make([]byte, len(data))
	for i := range data {
		out[i] = data[i] ^ secondMagic[i%len(secondMagic)] ^ baseMagic[i%len(baseMagic)]
	}
	return out
}

func checksum(data []byte) []byte {
	sum := crc64.Checksum(data, crc64.MakeTable(crc64.ECMA))
	return binary.LittleEndian.AppendUint64(nil, sum)
}

// wrapChecksum returns data with its masked checksum appended, masked, in one
// new buffer; data is not changed.
func wrapChecksum(data []byte) []byte {
	// Append the checksum masked from position zero.
	out := make([]byte, len(data)+8)
	copy(out, data)
	sum := out[len(data):]
	binary.LittleEndian.PutUint64(sum, crc64.Checksum(data, crc64.MakeTable(crc64.ECMA)))
	xorInPlace(sum)

	// Mask the whole buffer.
	xorInPlace(out)
	return out
}

// xorInPlace masks data in place as xor does.
func xorInPlace(data []byte) {
	for i := range data {
		data[i] ^= secondMagic[i%len(secondMagic)] ^ baseMagic[i%len(baseMagic)]
	}
}

func unwrapChecksum(data []byte) ([]byte, bool) {
	// Reject payloads too short to hold a value and its checksum.
	if len(data) < 9 {
		return nil, false
	}
	data = xor(data)
	csum := xor(data[len(data)-8:])
	data = data[:len(data)-8]
	outSum := checksum(data)
	return data, bytes.Equal(outSum, csum)
}

// EncodePackedMessage encodes a message to base64 with a checksum.
func EncodePackedMessage(message []byte) string {
	return base64.RawURLEncoding.EncodeToString(wrapChecksum(message))
}

// AppendPackedMessage appends the encoding EncodePackedMessage returns to dst,
// without the copy a string conversion makes.
func AppendPackedMessage(dst, message []byte) []byte {
	return base64.RawURLEncoding.AppendEncode(dst, wrapChecksum(message))
}

// DecodePackedMessage decodes the given ciphertext.
func DecodePackedMessage(message string) ([]byte, bool) {
	// Decode the URL-safe base64 message before verifying its checksum.
	out, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(message))
	if err != nil {
		return nil, false
	}
	out, ok := unwrapChecksum(out)
	if !ok {
		return nil, false
	}
	return out, true
}

// FindPackedMessages finds any packed messages in the given body of text.
func FindPackedMessages(text string) (packedMsgs [][]byte, packedMsgsSrc []string) {
	msgs := strings.Fields(text)
	out := make([][]byte, 0, len(msgs))
	for _, msg := range msgs {
		dec, ok := DecodePackedMessage(msg)
		if ok {
			out = append(out, dec)
			packedMsgsSrc = append(packedMsgsSrc, msg)
		}
	}
	return out, packedMsgsSrc
}
