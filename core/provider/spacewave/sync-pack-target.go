//go:build !js

package provider_spacewave

// syncFlushMaxPackBytes is the native sync target for one upload body. Native
// builds hold two bodies at a time, one packing and one uploading, so a large
// target cuts request count and manifest rows without memory pressure. It stays
// well under the writer.DefaultMaxPackBytes wire cap.
const syncFlushMaxPackBytes int64 = 32 * 1024 * 1024
