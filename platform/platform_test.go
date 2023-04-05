package bldr_platform

import "testing"

func TestParsePlatform(t *testing.T) {
	if _, err := ParsePlatform("unknown/platform"); err == nil {
		t.Fail()
	}

	p, err := ParsePlatform("native/windows/armv6")
	if err != nil {
		t.Fatal(err.Error())
	}
	_, ok := p.(*NativePlatform)
	if !ok {
		t.Fail()
	}

	p, err = ParsePlatform("web")
	if err != nil {
		t.Fatal(err.Error())
	}
	_, ok = p.(*WebPlatform)
	if !ok {
		t.Fail()
	}

	p, err = ParsePlatform("web/invalid/params")
	if err == nil {
		t.Fail()
	}
}
