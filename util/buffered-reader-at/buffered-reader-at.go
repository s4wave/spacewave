package buffered_reader_at

import (
	"io"
	"slices"
	"sort"
	"sync"
)

// TODO: cache eviction, cache budget, time based eviction / access count decay tick

// BufferedReaderAt enhances io.ReaderAt with caching, page alignment, and concurrency.
type BufferedReaderAt struct {
	reader   io.ReaderAt
	pageSize int64

	// mtx guards below fields
	mtx   sync.Mutex
	cache []*cacheRange
}

// cacheRange represents a range of cached data
type cacheRange struct {
	offset int64         // The start offset of the cache range
	size   int64         // The size of the cache range
	data   []byte        // The actual cached data
	done   chan struct{} // A channel to signal the completion of the read operation
	err    error         // Any error encountered during the read
}

// NewBufferedReaderAt creates a new BufferedReaderAt with the specified page size
func NewBufferedReaderAt(reader io.ReaderAt, pageSize int64) *BufferedReaderAt {
	if pageSize <= 0 {
		pageSize = 4096 // Default page size
	}
	return &BufferedReaderAt{
		reader:   reader,
		pageSize: pageSize,
		cache:    make([]*cacheRange, 0),
	}
}

// alignOffset adjusts the offset to align with the previous page boundary
func (br *BufferedReaderAt) alignOffset(offset int64) int64 {
	return (offset / br.pageSize) * br.pageSize
}

// ReadAt reads from the underlying io.ReaderAt into p, implementing caching, page alignment, and preventing overlapping cache pages.
// Supports concurrent requests.
func (br *BufferedReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	remaining := len(p)

	for remaining > 0 {
		currentOffset := off + int64(n)
		startOffset := br.alignOffset(currentOffset)
		endOffset := currentOffset + int64(remaining)
		if endOffset%br.pageSize != 0 {
			endOffset = br.alignOffset(endOffset) + br.pageSize
		}
		desiredSize := endOffset - startOffset

		br.mtx.Lock()

		// Using binary search to find the insertion index or existing range
		insertIndex := sort.Search(len(br.cache), func(i int) bool {
			return br.cache[i].offset >= startOffset
		})

		var matchedRange *cacheRange
		if insertIndex < len(br.cache) && br.cache[insertIndex].offset == startOffset {
			matchedRange = br.cache[insertIndex]
		} else {
			// Adjust the size of the new range if it overlaps with the next one
			if insertIndex < len(br.cache) {
				nextRange := br.cache[insertIndex]
				if startOffset+desiredSize > nextRange.offset {
					desiredSize = nextRange.offset - startOffset
				}
			}
			matchedRange = &cacheRange{
				offset: startOffset,
				size:   desiredSize,
				data:   make([]byte, desiredSize),
				done:   make(chan struct{}),
			}
			br.cache = slices.Insert(br.cache, insertIndex, matchedRange)

			go func(nr *cacheRange) {
				readN, readErr := br.reader.ReadAt(nr.data, nr.offset)
				if readN < len(nr.data) {
					nr.data = nr.data[:readN]
				}
				nr.size = int64(len(nr.data))
				nr.err = readErr
				close(nr.done)

				if nr.size == 0 {
					br.mtx.Lock()
					idx := slices.Index(br.cache, nr)
					if idx >= 0 {
						br.cache = slices.Delete(br.cache, idx, idx+1)
					}
					br.mtx.Unlock()
				}
			}(matchedRange)
		}
		br.mtx.Unlock()

		<-matchedRange.done // Wait for the read to complete

		if matchedRange.err != nil && (matchedRange.err != io.EOF || matchedRange.size == 0) {
			return n, matchedRange.err // Return the error if any
		}

		// Calculate the end of the desired read within the cached range, relative to the start of the file.
		desiredReadEnd := currentOffset + int64(remaining)
		// Calculate `copyEnd` relative to the start of `matchedRange.data`
		copyStart := max(currentOffset, matchedRange.offset) - matchedRange.offset
		copyEnd := min(desiredReadEnd, matchedRange.offset+matchedRange.size) - matchedRange.offset
		copied := copy(p[n:], matchedRange.data[copyStart:copyEnd])
		n += copied
		remaining -= copied

		if matchedRange.err == io.EOF && copied < remaining {
			// If EOF is encountered and we copied less than requested, stop reading.
			break
		}
	}

	// successful read
	if err == io.EOF && n == len(p) {
		err = nil
	}

	return n, err
}

// _ is a type assertion
var _ io.ReaderAt = ((*BufferedReaderAt)(nil))
