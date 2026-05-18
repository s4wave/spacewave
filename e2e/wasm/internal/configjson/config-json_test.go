//go:build !js

package configjson

import "testing"

type jsonMarshaler []byte

func (m jsonMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(m), nil
}

func TestMarshalCanonicalSortsObjectsRecursively(t *testing.T) {
	got, err := MarshalCanonical(jsonMarshaler(`{"z":2,"nested":{"b":true,"a":[{"d":"x","c":1}]},"a":null}`))
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"a":null,"nested":{"a":[{"c":1,"d":"x"}],"b":true},"z":2}`
	if string(got) != want {
		t.Fatalf("canonical json mismatch:\ngot:  %s\nwant: %s", got, want)
	}
}
