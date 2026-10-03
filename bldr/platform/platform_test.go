package bldr_platform

import "testing"

func TestParsePlatform(t *testing.T) {
	// Verify that platform parsing rejects an unknown target family.
	if _, err := ParsePlatform("unknown/platform"); err == nil {
		t.Fail()
	}

	// Verify that a desktop target resolves to a native platform.
	p, err := ParsePlatform("desktop/windows/armv6")
	if err != nil {
		t.Fatal(err.Error())
	}
	_, ok := p.(*NativePlatform)
	if !ok {
		t.Fail()
	}

	// Verify that the JavaScript target resolves to a JavaScript platform.
	p, err = ParsePlatform("js")
	if err != nil {
		t.Fatal(err.Error())
	}
	_, ok = p.(*JsPlatform)
	if !ok {
		t.Fail()
	}

	// Verify that JavaScript platform parsing rejects extra components.
	_, err = ParsePlatform("js/invalid/params")
	if err == nil {
		t.Fail()
	}

	// Verify that the web WebAssembly target resolves to a native platform.
	p, err = ParsePlatform("web/js/wasm")
	if err != nil {
		t.Fatal(err.Error())
	}
	_, ok = p.(*NativePlatform)
	if !ok {
		t.Fail()
	}

	// Verify that the WASI WebAssembly target resolves to a native platform.
	p, err = ParsePlatform("desktop/wasi/wasm")
	if err != nil {
		t.Fatal(err.Error())
	}
	_, ok = p.(*NativePlatform)
	if !ok {
		t.Fail()
	}
	_ = p
}
