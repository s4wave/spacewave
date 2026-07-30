package kvtx

import "bytes"

// PrefixSuccessor returns the smallest key above the contiguous prefix range.
// The boolean is false when prefix is empty or has no finite successor.
func PrefixSuccessor(prefix []byte) ([]byte, bool) {
	if len(prefix) == 0 {
		return nil, false
	}

	successor := bytes.Clone(prefix)
	for idx := len(successor) - 1; idx >= 0; idx-- {
		if successor[idx] != 0xff {
			successor[idx]++
			return successor[:idx+1], true
		}
	}
	return nil, false
}
