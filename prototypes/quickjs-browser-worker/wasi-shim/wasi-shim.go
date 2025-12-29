package wasishim

import "embed"

// WASIShim contains the bundled wasi-shim ES module.
//
//go:generate go run -v ./gen/main.go
//go:embed wasi-shim.esm.js
var WASIShim embed.FS
