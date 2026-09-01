package assetpack

import "io"

// ReaderAt joins physical asset pack parts into one logical random-access file.
type ReaderAt struct {
	parts []readerPart
	size  int64
}

type readerPart struct {
	reader io.ReaderAt
	offset int64
	size   int64
}

// NewReaderAt constructs a logical random-access file from ordered parts.
func NewReaderAt(parts []Part, readers []io.ReaderAt) (*ReaderAt, error) {
	if len(parts) != len(readers) {
		return nil, io.ErrUnexpectedEOF
	}
	r := &ReaderAt{}
	for i, part := range parts {
		if part.Size <= 0 {
			return nil, io.ErrUnexpectedEOF
		}
		r.parts = append(r.parts, readerPart{reader: readers[i], offset: r.size, size: part.Size})
		r.size += part.Size
	}
	return r, nil
}

// Size returns the logical file size.
func (r *ReaderAt) Size() int64 { return r.size }

// ReadAt reads across physical part boundaries.
func (r *ReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, io.ErrUnexpectedEOF
	}
	total := 0
	for _, part := range r.parts {
		if off >= part.offset+part.size {
			continue
		}
		if off < part.offset {
			off = part.offset
		}
		partOff := off - part.offset
		want := min(int64(len(p)-total), part.size-partOff)
		n, err := part.reader.ReadAt(p[total:total+int(want)], partOff)
		total += n
		off += int64(n)
		if err != nil && err != io.EOF {
			return total, err
		}
		if n != int(want) {
			return total, io.ErrUnexpectedEOF
		}
		if total == len(p) {
			return total, nil
		}
	}
	return total, io.EOF
}
