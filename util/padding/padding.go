package padding

import (
	"github.com/pkg/errors"
)

// PadInPlace attempts to extend data out to 32 byte intervals.
// Appends a 1-byte trailer with the padding length.
func PadInPlace(data []byte) []byte {
	var paddingLen byte
	dataLen := len(data) + 1 // for extra padding length byte
	if dlm := dataLen % 32; dlm != 0 {
		paddingLen = byte(32 - dlm)
	}
	nlen := dataLen + int(paddingLen)
	if cap(data) >= nlen {
		data = data[:nlen] // extend slice with existing capacity
		for i := dataLen - 1; i < len(data)-1; i++ {
			data[i] = 0
		}
		data[len(data)-1] = paddingLen
	} else {
		og := data
		data = make([]byte, nlen) // zeroed by golang
		copy(data, og)
		data[len(data)-1] = paddingLen
		// original buffer is released
	}
	return data
}

// UnpadInPlace removes padding according to the appended length byte.
func UnpadInPlace(data []byte) ([]byte, error) {
	paddingLen := int(data[len(data)-1])
	if paddingLen >= len(data) || paddingLen > 32 || paddingLen < 0 {
		return nil, errors.Errorf(
			"%d padding indicated but message is %d bytes",
			paddingLen,
			len(data),
		)
	}
	data = data[:len(data)-paddingLen-1]
	return data, nil
}
