package bldr_platform

import "testing"

func TestParseCloudflarePlatform(t *testing.T) {
	if _, err := ParsePlatform("cloudflare"); err == nil {
		t.Fatal("expected ambiguous cloudflare platform id to fail")
	}

	p, err := ParsePlatform("cloudflare-workers")
	if err != nil {
		t.Fatal(err.Error())
	}
	plat, ok := p.(*CloudflarePlatform)
	if !ok {
		t.Fatal("expected *CloudflarePlatform")
	}
	if plat.GetPlatformID() != PlatformID_CLOUDFLARE {
		t.Fatalf("unexpected platform id: %s", plat.GetPlatformID())
	}
	if plat.GetBasePlatformID() != PlatformID_CLOUDFLARE {
		t.Fatalf("unexpected base platform id: %s", plat.GetBasePlatformID())
	}
	if ext := plat.GetExecutableExt(); ext != ".mjs" {
		t.Fatalf("unexpected executable ext: %s", ext)
	}
	if plat.GetInputPlatformID() != "cloudflare-workers" {
		t.Fatalf("unexpected input platform id: %s", plat.GetInputPlatformID())
	}

	if _, err := ParsePlatform("cloudflare-workers/extra"); err == nil {
		t.Fatal("expected unrecognized suffix to fail")
	}
}
