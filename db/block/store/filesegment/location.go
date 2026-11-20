package block_store_filesegment

// Location is a byte range of a file whose encoded bytes were a block when the
// file was indexed. ModTime and Size are the file's stat at index time.
type Location struct {
	// Path is the file's absolute path on this Device.
	Path string
	// Offset is the range's first byte in the file.
	Offset int64
	// Length is the range's length in bytes.
	Length int64
	// ModTime is the file's modification time at index time, in Unix ns.
	ModTime int64
	// Size is the file's size at index time.
	Size int64
}
