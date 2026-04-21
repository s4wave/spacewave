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

func wrapChecksum(data []byte) []byte {
	return xor(append(data, xor(checksum(data))...))
}

func unwrapChecksum(data []byte) ([]byte, bool) {
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

// DecodePackedMessage decodes the given ciphertext.
func DecodePackedMessage(message string) ([]byte, bool) {
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
