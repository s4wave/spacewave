//go:build js

package provider_spacewave

// deviceTypeValue is the device type sent in X-Device-Type headers.
// Browser/WASM builds do not send a device type (Turnstile is used instead).
const deviceTypeValue = ""
