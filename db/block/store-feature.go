package block

const (
	// StoreFeatureNativeBatchPut means PutBlockBatch is implemented natively.
	StoreFeatureNativeBatchPut = StoreFeature_STORE_FEATURE_NATIVE_BATCH_PUT
	// StoreFeatureNativeBatchExists means GetBlockExistsBatch is implemented natively.
	StoreFeatureNativeBatchExists = StoreFeature_STORE_FEATURE_NATIVE_BATCH_EXISTS
	// StoreFeatureNativeBackgroundPut means PutBlockBackground actually deprioritizes writes.
	StoreFeatureNativeBackgroundPut = StoreFeature_STORE_FEATURE_NATIVE_BACKGROUND_PUT
	// StoreFeatureNativeFlush means Flush has buffered work to publish.
	StoreFeatureNativeFlush = StoreFeature_STORE_FEATURE_NATIVE_FLUSH
	// StoreFeatureNativeDeferFlush means BeginDeferFlush and EndDeferFlush batch flush work.
	StoreFeatureNativeDeferFlush = StoreFeature_STORE_FEATURE_NATIVE_DEFER_FLUSH
	// StoreFeatureSelfBuffered means the store owns its background write buffer
	// and Sync barrier, so the world block engine must not wrap it in a
	// BufferedStore. Non-self-buffered native KV stores (bbolt, badger) get a
	// deferred-mode BufferedStore instead.
	StoreFeatureSelfBuffered = StoreFeature_STORE_FEATURE_SELF_BUFFERED
)

// Has reports whether this feature bitset contains every requested feature.
func (s StoreFeature) Has(feat StoreFeature) bool {
	return s&feat == feat
}
