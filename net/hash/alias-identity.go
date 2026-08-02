package hash

// AliasIdentityToken is an opaque per-instance token used to detect
// in-memory block aliases while marshaling a transaction.
type AliasIdentityToken = []byte

// BlockAliasIdentity returns the in-memory alias token for Hash.
func (h *Hash) BlockAliasIdentity() *AliasIdentityToken {
	return &h.unknownFields
}
