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
	if pageSize <= 0 {
		return 0
	}
	pageIdxValue := off / uint64(pageSize)
	if pageIdxValue > uint64(len(pages)) {
		return 0
	}
	pageIdx := int(pageIdxValue)           //nolint:gosec // the page-count check bounds this conversion to the slice index range.
	pageOff := int(off % uint64(pageSize)) //nolint:gosec // the remainder is strictly less than the positive int page size.
	written := 0
	for written < len(dst) && pageIdx < len(pages) {
		n := copy(dst[written:], pages[pageIdx][pageOff:])
		written += n
		pageIdx++
		pageOff = 0
	}
	return written
}
