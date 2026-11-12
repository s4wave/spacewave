package web_entrypoint_index

import (
	"regexp"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
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

func TestImportMapWithWebPkgFallback(t *testing.T) {
	// A map without the shared prefix gains the service worker fallback.
	im := ImportMap{Imports: map[string]string{"test": "./test.mjs"}}
	got := im.WithWebPkgFallback()
	if got.Imports["bldr-web-pkg/"] != "/b/pkg/" {
		t.Errorf("fallback binding = %q, want /b/pkg/", got.Imports["bldr-web-pkg/"])
	}
	if _, ok := im.Imports["bldr-web-pkg/"]; ok {
		t.Error("WithWebPkgFallback() modified the receiver")
	}

	// An explicit binding, such as a pinned package base, is kept.
	pinned := ImportMap{Imports: map[string]string{"bldr-web-pkg/": "/b/pa/web/manifest/root/pkgs/"}}
	if got := pinned.WithWebPkgFallback().Imports["bldr-web-pkg/"]; got != "/b/pa/web/manifest/root/pkgs/" {
		t.Errorf("explicit binding = %q, want the pinned base", got)
	}
}

func TestRenderIndexHTML(t *testing.T) {
	// Prepare valid index data containing one import and an entrypoint.
	data := IndexData{
		ImportMap: ImportMap{
			Imports: map[string]string{
				"test": "./test.mjs",
			},
		},
		EntrypointPath: "./test/entry.mjs",
	}

	// Render the embedded index from the test data.
	result, err := RenderIndexHTML(data)
	if err != nil {
		t.Fatalf("RenderIndexHTML() error = %v", err)
	}

	// Check if result contains the expected content
	if !strings.Contains(result, `"test":"./test.mjs"`) {
		t.Error("RenderIndexHTML() result doesn't contain expected import map")
	}
	if !strings.Contains(result, `"bldr-web-pkg/":"/b/pkg/"`) {
		t.Error("RenderIndexHTML() result doesn't bind the shared web package prefix")
	}
	if !strings.Contains(result, "./test/entry.mjs") {
		t.Error("RenderIndexHTML() result doesn't contain expected entrypoint path")
	}
	if !strings.Contains(result, `name="darkreader-lock"`) {
		t.Error("RenderIndexHTML() result doesn't contain Dark Reader lock")
	}
	if !strings.Contains(result, `id="bldr-initial-loading-shell"`) {
		t.Error("RenderIndexHTML() result doesn't contain initial loading shell")
	}
	if !strings.Contains(result, `Spacewave`) {
		t.Error("RenderIndexHTML() result doesn't contain Spacewave brand copy")
	}
	if strings.Contains(result, `Loading app`) {
		t.Error("RenderIndexHTML() result contains raw Loading app text")
	}
}

func TestRenderIndexHTMLInvalidTemplate(t *testing.T) {
	// Temporarily modify the template to make it invalid
	originalHTML := indexHTML
	indexHTML = "<!doctype html>"
	defer func() { indexHTML = originalHTML }()

	// Require rendering to reject a template missing both markers.
	_, err := RenderIndexHTML(IndexData{})
	if err == nil {
		t.Error("RenderIndexHTML() with invalid template should return error")
	}
}

func TestRenderIndexHTMLEscapesEntrypointPath(t *testing.T) {
	result, err := RenderIndexHTML(IndexData{
		ImportMap:      ImportMap{Imports: map[string]string{}},
		EntrypointPath: `/boot.mjs?x="bad"&y=<tag>`,
	})
	if err != nil {
		t.Fatalf("RenderIndexHTML() error = %v", err)
	}
	if strings.Contains(result, `src="/boot.mjs?x="bad"&y=<tag>"`) {
		t.Fatalf("RenderIndexHTML() rendered unescaped entrypoint path: %s", result)
	}
	if !strings.Contains(result, `src="/boot.mjs?x=&#34;bad&#34;&amp;y=&lt;tag&gt;"`) {
		t.Fatalf("RenderIndexHTML() missing escaped entrypoint path: %s", result)
	}
}

// TestRenderIndexHTMLKeepsStaticImportMap checks the shell still carries the
// map in a static importmap tag, inert inside its template, since released
// web listeners extract that tag from the deployed shell, and that the React
// version reaches the inline script escaped.
func TestRenderIndexHTMLKeepsStaticImportMap(t *testing.T) {
	result, err := RenderIndexHTML(IndexData{
		ImportMap:      ImportMap{Imports: map[string]string{"react": "./pkgs/react/index.mjs"}},
		EntrypointPath: "/boot.mjs",
		ReactVersion:   `19.2.0"<`,
	})
	if err != nil {
		t.Fatalf("RenderIndexHTML() error = %v", err)
	}
	staticMap := regexp.MustCompile(`(?is)<template id="bldr-import-map"[^>]*>\s*<script\s+type=["']importmap["'][^>]*>\s*\{"imports":\{[^<]*"react":"./pkgs/react/index.mjs"[^<]*\}\}\s*</script>\s*</template>`)
	if !staticMap.MatchString(result) {
		t.Fatalf("RenderIndexHTML() missing the static import map in its template: %s", result)
	}
	if !strings.Contains(result, `data-react-version="19.2.0&#34;&lt;"`) {
		t.Fatalf("RenderIndexHTML() missing escaped React version: %s", result)
	}
	if strings.Count(result, `type="importmap"`) != 1 {
		t.Fatalf("RenderIndexHTML() rendered more than one static import map")
	}
}

func TestImportMapJSONMarshal(t *testing.T) {
	// Prepare one import map entry to verify its JSON representation.
	im := ImportMap{
		Imports: map[string]string{
			"react": "./pkgs/react/index.mjs",
		},
	}

	// Marshal the import map to its deterministic JSON form.
	rendered := im.String()

	// Parse the JSON and verify the stored import URL.
	var p fastjson.Parser
	v, err := p.Parse(rendered)
	if err != nil {
		t.Fatalf("parse import map JSON error = %v", err)
	}
	if got := string(v.GetStringBytes("imports", "react")); got != "./pkgs/react/index.mjs" {
		t.Errorf("import map value mismatch: got %q", got)
	}
}
