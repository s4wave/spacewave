//go:build !js

package provider_spacewave

// deviceTypeValue is the device type sent in X-Device-Type headers.
// Native builds (desktop, CLI) identify as "desktop".
const deviceTypeValue = "desktop"
