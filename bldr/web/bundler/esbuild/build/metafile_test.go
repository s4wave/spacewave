package bldr_web_bundler_esbuild_build

import "testing"

func TestParseEsbuildMetafile(t *testing.T) {
	// Parse an esbuild metafile with an input and JavaScript and CSS outputs.
	meta, err := ParseEsbuildMetafile([]byte(`{
		"inputs": {
			"src/index.ts": {
				"bytes": 12,
				"imports": [{"path": "./dep.ts"}]
			}
		},
		"outputs": {
			"dist/index.js": {
				"bytes": 34,
				"entryPoint": "src/index.ts",
				"cssBundle": "dist/index.css"
			},
			"dist/index.css": {
				"bytes": 56
			}
		}
	}`))
	if err != nil {
		t.Fatalf("ParseEsbuildMetafile() error = %v", err)
	}

	// Verify the source input record retains its byte count.
	input, ok := meta.Inputs["src/index.ts"]
	if !ok {
		t.Fatal("expected src/index.ts input")
	}
	if input.Bytes != 12 {
		t.Fatalf("expected input bytes 12, got %d", input.Bytes)
	}

	// Verify the JavaScript output retains its size, entry point, and CSS bundle.
	jsOutput, ok := meta.Outputs["dist/index.js"]
	if !ok {
		t.Fatal("expected dist/index.js output")
	}
	if jsOutput.Bytes != 34 {
		t.Fatalf("expected dist/index.js bytes 34, got %d", jsOutput.Bytes)
	}
	if jsOutput.EntryPoint != "src/index.ts" {
		t.Fatalf("expected entryPoint src/index.ts, got %q", jsOutput.EntryPoint)
	}
	if jsOutput.CssBundle != "dist/index.css" {
		t.Fatalf("expected cssBundle dist/index.css, got %q", jsOutput.CssBundle)
	}

	// Verify the CSS output retains its byte count.
	cssOutput, ok := meta.Outputs["dist/index.css"]
	if !ok {
		t.Fatal("expected dist/index.css output")
	}
	if cssOutput.Bytes != 56 {
		t.Fatalf("expected dist/index.css bytes 56, got %d", cssOutput.Bytes)
	}
}
