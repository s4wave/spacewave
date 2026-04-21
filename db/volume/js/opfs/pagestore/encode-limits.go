package pagestore

func mustUint16Len(v int) uint16 {
	if v < 0 || v > 0xffff {
		panic("pagestore: length overflows uint16")
	}
	return uint16(v)
}
