package blob

// buildMockRawBlob builds a new mock raw blob.
func buildMockRawBlob() *Blob {
	testBuf := []byte("test-raw-blob")
	return &Blob{
		BlobType:  BlobType_BlobType_RAW,
		TotalSize: uint64(len(testBuf)),
		RawData:   testBuf,
	}
}
