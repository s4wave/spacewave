package block

const (
	// StoreFeatureNativeBatchPut means PutBlockBatch is implemented natively.
	StoreFeatureNativeBatchPut = StoreFeature_STORE_FEATURE_NATIVE_BATCH_PUT
	// StoreFeatureNativeBatchExists means GetBlockExistsBatch is implemented natively.
	StoreFeatureNativeBatchExists = StoreFeature_STORE_FEATURE_NATIVE_BATCH_EXISTS
	// StoreFeatureSelfBuffered means the store has its own read-through pending
	// buffer, so the world block engine must not wrap it in a BufferedStore.
	// Volume.Sync remains the durability barrier for volume-backed stores.
	StoreFeatureSelfBuffered = StoreFeature_STORE_FEATURE_SELF_BUFFERED
)

// Has reports whether this feature bitset contains every requested feature.
func (s StoreFeature) Has(feat StoreFeature) bool {
	return s&feat == feat
}
