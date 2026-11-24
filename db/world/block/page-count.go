package world_block

// PageCount counts pages and their stored bytes.
type PageCount struct {
	// Pages is the number of pages.
	Pages uint64
	// Bytes is their stored length.
	Bytes int64
}
