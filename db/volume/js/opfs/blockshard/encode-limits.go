package blockshard

func mustUint16Len(v int) uint16 {
	if v < 0 || v > 0xffff {
		panic("blockshard: length overflows uint16")
	}
	return uint16(v)
}

func mustUint32Len(v int) uint32 {
	if v < 0 || uint64(v) > 0xffffffff {
		panic("blockshard: length overflows uint32")
	}
	return uint32(v) // #nosec G115 -- bounded above.
}
