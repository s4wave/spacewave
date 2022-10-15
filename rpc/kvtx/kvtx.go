package rpc_kvtx

// NewKeyRequest constructs a new KeyRequest with a key.
func NewKeyRequest(key []byte) *KvtxKeyRequest {
	return &KvtxKeyRequest{Key: key}
}
