package dex_solicit

// SetBlock copies a found block, with its refs and data, from a verified
// response. It does not copy Linked: a forwarded block is always sent as data.
func (m *DexMessage) SetBlock(found *DexMessage) {
	m.Found = true
	m.Data = found.GetData()
	m.Refs = found.GetRefs()
	m.RefsKnown = found.GetRefsKnown()
}
