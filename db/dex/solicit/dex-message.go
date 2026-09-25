package dex_solicit

// SetBlock copies a found block, with its refs, from a verified response.
func (m *DexMessage) SetBlock(found *DexMessage) {
	m.Found = true
	m.Data = found.GetData()
	m.Refs = found.GetRefs()
	m.RefsKnown = found.GetRefsKnown()
}
