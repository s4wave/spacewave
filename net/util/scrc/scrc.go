package scrc

import "hash/crc64"

// ecmaTable is the immutable crc64 ECMA table shared by all calls.
var ecmaTable = crc64.MakeTable(crc64.ECMA)

// Crc64 computes the crc64 of some data.
func Crc64(ds ...[]byte) uint64 {
	var sum uint64
	for _, d := range ds {
		sum = crc64.Update(sum, ecmaTable, d)
	}
	return sum
}
