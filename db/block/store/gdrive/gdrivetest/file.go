package gdrivetest

// file is a complete Drive file or an unpublished upload session.
type file struct {
	// id is the opaque file or session identity.
	id string
	// parent is the containing folder's identity.
	parent string
	// meta is the original create metadata.
	meta []byte
	// path holds completed bytes on disk for fixtures larger than memory.
	path string
	// storedSize is the completed media size.
	storedSize int
	// checksum is Drive's independently computed completed-media MD5.
	checksum string
	// finished marks an atomically published file, including an empty one.
	finished bool
	// data is the committed media, partial only for an upload session.
	data []byte
}
