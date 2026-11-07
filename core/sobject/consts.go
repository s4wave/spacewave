package sobject

const (
	// MaxBlockRefSize is the maximum size in bytes for block references.
	MaxBlockRefSize = 256

	// MaxInnerDataSize is the maximum size in bytes for inner data.
	MaxInnerDataSize = 1024 * 1024 // 1 MB

	// MaxBatchSize is the size in bytes at which a long-running write commits
	// its batch and continues in the next. It is half of MaxInnerDataSize,
	// leaving room for the entries the size accounting does not count and for
	// the operation envelope.
	MaxBatchSize = MaxInnerDataSize / 2

	// MaxOperations is the maximum number of operations above the checkpoint.
	MaxOperations = 65536

	// MaxStateDataSize is the maximum size in bytes for state data.
	MaxStateDataSize = 10 * 1024 * 1024 // 10 MB

	// MaxSyncStateSize is the largest encoded state, in bytes, that one sync
	// frame carries. A peer can neither send nor import a larger state.
	MaxSyncStateSize = 10 * 1024 * 1024 // 10 MB
)
