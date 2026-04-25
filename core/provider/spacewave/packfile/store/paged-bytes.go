package store

import "bytes"

// buildPagedBytes splits fetched bytes into fixed-size pages.
func buildPagedBytes(pageSize int, data []byte) [][]byte {
	var pages [][]byte
	for len(data) > 0 {
		n := min(len(data), pageSize)
		pages = append(pages, bytes.Clone(data[:n]))
		data = data[n:]
	}
	return pages
}

// copyPagedBytes copies bytes starting at off from paged storage into dst.
func copyPagedBytes(dst []byte, pages [][]byte, pageSize int, off uint64) int {
	pageIdx := int(off / uint64(pageSize))
	pageOff := int(off % uint64(pageSize))
	written := 0
	for written < len(dst) && pageIdx < len(pages) {
		n := copy(dst[written:], pages[pageIdx][pageOff:])
		written += n
		pageIdx++
		pageOff = 0
	}
	return written
}
