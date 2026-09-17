package kvtx_block_okra

// clonePageEntries gives a new page independent, mutable entries while packing
// their headers and byte fields into page-local allocations. Rebuilt pages must
// not share Entry objects, inline blobs or refs: retained cursors may still read
// or update the old page. This replaces allocation layout, not CloneVT semantics
// or the durable encoding. Call only with the new page's private pointer slice.
func clonePageEntries(entries []*Entry) {
	bytesNeeded := 0
	for _, entry := range entries {
		if entry != nil {
			bytesNeeded += len(entry.Key) + len(entry.Hash) + len(entry.unknownFields)
		}
	}
	owned := make([]Entry, len(entries))
	storage := make([]byte, bytesNeeded)
	copyBytes := func(src []byte) []byte {
		if src == nil {
			return nil
		}
		n := len(src)
		dst := storage[:n:n]
		copy(dst, src)
		storage = storage[n:]
		return dst
	}
	for i, src := range entries {
		if src == nil {
			continue
		}
		dst := &owned[i]
		*dst = *src
		dst.Key = copyBytes(src.Key)
		dst.Hash = copyBytes(src.Hash)
		dst.unknownFields = copyBytes(src.unknownFields)
		dst.ChildRef = src.ChildRef.CloneVT()
		dst.ValueRef = src.ValueRef.CloneVT()
		dst.ValueBlob = src.ValueBlob.CloneVT()
		entries[i] = dst
	}
}
