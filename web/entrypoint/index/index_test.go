package web_entrypoint_index

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestImportMapString(t *testing.T) {
	im := ImportMap{
		Imports: map[string]string{
			"test": "./test.mjs",
		},
	}

	expected := `{"imports":{"test":"./test.mjs"}}`
	if got := im.String(); got != expected {
		t.Errorf("ImportMap.String() = %v, want %v", got, expected)
	}
}

func TestRenderIndexHTML(t *testing.T) {
	data := IndexData{
		ImportMap: ImportMap{
			Imports: map[string]string{
				"test": "./test.mjs",
			},
		},
		EntrypointPath: "./test/entry.mjs",
	}

	result, err := RenderIndexHTML(data)
	if err != nil {
		t.Fatalf("RenderIndexHTML() error = %v", err)
	}

	// Check if result contains the expected content
	if !strings.Contains(result, `"test":"./test.mjs"`) {
		t.Error("RenderIndexHTML() result doesn't contain expected import map")
	}
	if !strings.Contains(result, "./test/entry.mjs") {
		t.Error("RenderIndexHTML() result doesn't contain expected entrypoint path")
	}
}

func TestRenderIndexHTMLInvalidTemplate(t *testing.T) {
	// Temporarily modify the template to make it invalid
	originalHTML := indexHTML
	indexHTML = "{{.InvalidTemplate}}"
	defer func() { indexHTML = originalHTML }()

	_, err := RenderIndexHTML(IndexData{})
	if err == nil {
		t.Error("RenderIndexHTML() with invalid template should return error")
	}
}

func TestImportMapJSONMarshal(t *testing.T) {
	im := ImportMap{
		Imports: map[string]string{
			"react": "./pkgs/react/index.mjs",
		},
	}

	// Test direct JSON marshaling
	b, err := json.Marshal(im)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Verify the JSON structure
	var unmarshaled ImportMap
	if err := json.Unmarshal(b, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if unmarshaled.Imports["react"] != "./pkgs/react/index.mjs" {
		t.Error("JSON marshaling/unmarshaling didn't preserve import map values")
	}
}
