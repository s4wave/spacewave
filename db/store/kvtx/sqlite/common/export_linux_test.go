package common

// SetPunchDiskFree replaces how punch rounds read the disk's free space
// until the returned func restores it.
func SetPunchDiskFree(free func(path string) (int64, error)) (restore func()) {
	prev := punchDiskFree
	punchDiskFree = free
	return func() { punchDiskFree = prev }
}
