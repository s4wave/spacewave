//go:build !linux

package s4db

// bgDirtyMax is the most a background job writes between pushes of its bytes
// to the drive. A push here flushes the whole file and the drive's cache, so it
// batches 8 MiB rather than pay that flush for every chunk.
const bgDirtyMax = 8 << 20
