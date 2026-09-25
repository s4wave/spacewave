//go:build js

package provider_spacewave

// syncFlushMaxPackBytes is the browser sync target for one upload body. Browser
// builds need headroom for two bodies (one packing, one uploading), the request
// body copy, foreground uploads, OPFS, and the full app runtime.
const syncFlushMaxPackBytes int64 = 4 * 1024 * 1024
