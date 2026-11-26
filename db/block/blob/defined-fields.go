package blob

// DefinedFieldsOnly reports whether the blob and every message below it that
// the blob package defines (its chunk index, the index's chunker args and
// their rabin and jc args, and each chunk) hold only fields their types
// define, so that decoding kept no unknown field. A chunk's data_ref is a
// block.BlockRef from another package and is not checked. A nil chunk counts
// as unknown.
//
// Any bytes decode as some Blob, because protobuf skips fields it does not
// know. A decoder that guessed the Blob type for a block it was not told the
// type of can therefore read a block of another type as a valid Blob, and the
// fields of the real type, including the refs it names, land in unknownFields.
// Such a decoder needs this check before it trusts the refs the Blob names.
func (b *Blob) DefinedFieldsOnly() bool {
	return len(b.unknownFields) == 0 && b.GetChunkIndex().definedFieldsOnly()
}

// definedFieldsOnly reports DefinedFieldsOnly for an index; nil has none.
func (r *ChunkIndex) definedFieldsOnly() bool {
	// Check the index itself and its chunker args.
	if r == nil {
		return true
	}
	if len(r.unknownFields) != 0 || !r.GetChunkerArgs().definedFieldsOnly() {
		return false
	}

	// Check each chunk, refusing a nil one.
	for _, c := range r.GetChunks() {
		if c == nil || len(c.unknownFields) != 0 {
			return false
		}
	}
	return true
}

// definedFieldsOnly reports DefinedFieldsOnly for chunker args; nil has none.
func (c *ChunkerArgs) definedFieldsOnly() bool {
	if c == nil {
		return true
	}
	return len(c.unknownFields) == 0 &&
		(c.GetRabinArgs() == nil || len(c.GetRabinArgs().unknownFields) == 0) &&
		(c.GetJcArgs() == nil || len(c.GetJcArgs().unknownFields) == 0)
}
