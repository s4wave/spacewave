package kvtx_block_okra

// UnmarshalDecodedBlock decodes a page read through a block cursor with the
// generated arena decoder, so a page costs a fixed small number of allocations
// instead of about six per entry.
func (p *Page) UnmarshalDecodedBlock(data []byte) error {
	return p.UnmarshalVTArena(data)
}
