package sobject

const (
	// MaxErrorDetailsSize is the maximum size in bytes for error details
	MaxErrorDetailsSize = 4096

	// MaxBlockRefSize is the maximum size in bytes for block references
	MaxBlockRefSize = 256

	// MaxInnerDataSize is the maximum size in bytes for inner data
	MaxInnerDataSize = 1024 * 1024 // 1 MB

	// MaxSignatureSize is the maximum size in bytes for signatures
	MaxSignatureSize = 512

	// MaxParticipants is the maximum number of participants in a shared object
	MaxParticipants = 100

	// MaxOperations is the maximum number of pending operations
	MaxOperations = 1000

	// MaxOperationRejections is the maximum number of operation rejections
	MaxOperationRejections = 1000

	// MaxValidatorSignatures is the maximum number of validator signatures
	MaxValidatorSignatures = 100

	// MaxStateDataSize is the maximum size in bytes for state data
	MaxStateDataSize = 10 * 1024 * 1024 // 10 MB
)
