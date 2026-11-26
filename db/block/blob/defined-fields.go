package blob

// DefinedFieldsOnly reports whether the blob, its chunk index and each chunk
// hold only fields their types define, so that decoding kept no unknown field.
//
// Any bytes decode as some Blob, because protobuf skips fields it does not
// know. A decoder that guessed the Blob type for a block it was not told the
// type of can therefore read a block of another type as a valid Blob, and the
// fields of the real type, including the refs it names, land in unknownFields.
// A guess is safe only when this returns true. A nil chunk counts as unknown.
func (b *Blob) DefinedFieldsOnly() bool {
	// Check the blob, then its chunk index and every chunk when it has one.
	if len(b.unknownFields) != 0 {
		return false
	}
	index := b.GetChunkIndex()
	if index == nil {
		return true
	}
	if len(index.unknownFields) != 0 {
		return false
	}
	for _, c := range index.GetChunks() {
		if c == nil || len(c.unknownFields) != 0 {
			return false
		}
	}
	return true
}
