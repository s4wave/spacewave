package forge_runtime

// OwnedWorkerCapacity pairs an owned capacity record with the Forge Worker
// object key it describes, so scans can name workers for reclaim.
type OwnedWorkerCapacity struct {
	// WorkerObjectKey is the Forge Worker object key of the record.
	WorkerObjectKey string
	// Capacity is the owned capacity record.
	Capacity *WorkerCapacity
}
