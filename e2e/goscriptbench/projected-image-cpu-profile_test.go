//go:build !js

package goscriptbench

import "testing"

func TestMarshalProjectedImageCPUProfile(t *testing.T) {
	data, err := marshalProjectedImageCPUProfile(map[string]any{
		"nodes":     []any{map[string]any{"id": 1.0}},
		"endTime":   2.5,
		"startTime": 1.25,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	want := "{\"endTime\":2.5,\"nodes\":[{\"id\":1}],\"startTime\":1.25}\n"
	if string(data) != want {
		t.Fatalf("CPU profile JSON = %s, want %s", data, want)
	}
	if _, err := marshalProjectedImageCPUProfile(make(chan struct{})); err == nil {
		t.Fatal("unsupported CPU profile value encoded")
	}
}
