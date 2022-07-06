package demo

// _ enables embedding
import _ "embed"

// generate the js
//go:generate ../../hack/bin/esbuild test-component.tsx --bundle --minify --format=esm --outfile=test-component.js

// TestComponentJS is the JS for the Demo react component.
//go:embed test-component.js
var TestComponentJS string
