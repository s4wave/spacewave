package kvtx

// HealthSizes is a measurement of a volume's storage.
type HealthSizes struct {
	// FileBytes is the length of the storage files.
	FileBytes uint64
	// LiveValueBytes is the length of the live values the files hold.
	LiveValueBytes uint64
	// FreeDiskBytes is the disk space free to the process.
	FreeDiskBytes uint64
	// ReserveBytes is the free disk below which the volume refuses new
	// writes, or zero for no reserve.
	ReserveBytes uint64
}
