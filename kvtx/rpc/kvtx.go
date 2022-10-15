package kvtx_rpc

// NewKeyRequest constructs a new KeyRequest with a key.
func NewKeyRequest(key []byte) *KvtxKeyRequest {
	return &KvtxKeyRequest{Key: key}
}
