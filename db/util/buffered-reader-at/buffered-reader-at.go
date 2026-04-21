package buffered_reader_at

import (
	"errors"
	"io"
	"slices"
	"sort"
	"sync"
)

// SliceReaderAt supports reading data at a larger range than the buffer provided to ReadAt.
type SliceReaderAt interface {
	// SliceReadAt reads a slice of data from the requested location.
	// NOTE: the returned slice may start before or after the requested location and length.
	// NOTE: this may return a completely different range than what you asked for!
	SliceReadAt(offset, length int64) (dataOffset int64, data []byte, err error)
}

// BufferedReaderAt enhances io.ReaderAt with caching, page alignment, and concurrency.
//
// TODO: implement time and size based cache eviction, memory budget
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

		var matchedRange *cacheRange = nil

		if insertIndex > 0 && br.cache[insertIndex-1].offset+br.cache[insertIndex-1].size > currentOffset {
			// Case: the previous range before insertIndex encompasses currentOffset.
			matchedRange = br.cache[insertIndex-1]
		} else if insertIndex < len(br.cache) && br.cache[insertIndex].offset == startOffset {
			// Case: the matched insertIndex contains the offset.
			matchedRange = br.cache[insertIndex]
		}

		if matchedRange == nil {
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
				done:   make(chan struct{}),
			}
			br.cache = slices.Insert(br.cache, insertIndex, matchedRange)

			go func(nr *cacheRange) {
				// If this is a DynamicReaderAt we can request the data and keep the entire result.
				// For HTTP fetchers, this is used to handle status 200 when 206 is expected.
				var data []byte
				var readErr error
				sliceReader, sliceReaderOk := br.reader.(SliceReaderAt)
				if sliceReaderOk {
					readDataOffset, readData, err := sliceReader.SliceReadAt(nr.offset, nr.size)
					if err != nil {
						readErr = err
					}
					if len(readData) != 0 {
						// Adjust the offset and size according to the returned offset.
						// The size can be adjusted w/o a mutex lock but the offset requires a sort and mtx lock.
						if readDataOffset != nr.offset {
							br.mtx.Lock()
							nr.offset = readDataOffset
							slices.SortFunc(br.cache, func(a, b *cacheRange) int {
								return int(a.offset - b.offset)
							})
							br.mtx.Unlock()
						}
					}
					data = readData
				}

				// if !sliceReaderOk or if the slice reader returned len(0) try ReadAt
				if len(data) == 0 {
					data = make([]byte, nr.size)
					var readN int
					readN, readErr = br.reader.ReadAt(data, nr.offset)
					data = data[:min(readN, len(data))]
				}

				if len(data) != 0 { // avoid keeping a reference to the slice capacity
					nr.size = int64(len(data))
					nr.data = data
				}
				if readErr != nil && (readErr != io.EOF || len(data) == 0) {
					nr.err = readErr
				}
				close(nr.done)

				// If the size was zero, drop the range.
				// (The calls waiting on the read will still get the readErr).
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

		if matchedRange.err != nil {
			return n, matchedRange.err // Return the error if any
		}

		copyStart := int(currentOffset - matchedRange.offset)
		if copyStart < 0 {
			// range returned was after the requested starting point
			return n, errors.New("incorrect range of data returned")
		}
		if len(matchedRange.data) <= copyStart {
			return n, errors.New("shorter range of data returned than expected")
		}

		copied := copy(p[n:], matchedRange.data[copyStart:])
		n += copied
		remaining -= copied
	}

	return n, err
}

// _ is a type assertion
var _ io.ReaderAt = ((*BufferedReaderAt)(nil))
