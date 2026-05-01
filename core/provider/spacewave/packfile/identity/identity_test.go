package identity

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
)

func TestBuildPackIDIsDeterministic(t *testing.T) {
	result := &writer.PackResult{
		SortedKeyDigest:  bytes.Repeat([]byte{1}, 32),
		PackBytesDigest:  bytes.Repeat([]byte{2}, 32),
		PolicyTag:        PolicyTag(writer.DefaultPolicy()),
		ValueOrderPolicy: ValueOrderIterator,
	}

	first, err := BuildPackID("resource-a", result)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPackID("resource-a", result)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("pack id changed: %q != %q", first, second)
	}
	if err := ValidatePackID(first); err != nil {
		t.Fatal(err)
	}
}

func TestBuildPackIDBindsResourceAndBytes(t *testing.T) {
	result := &writer.PackResult{
		SortedKeyDigest:  bytes.Repeat([]byte{1}, 32),
		PackBytesDigest:  bytes.Repeat([]byte{2}, 32),
		PolicyTag:        PolicyTag(writer.DefaultPolicy()),
		ValueOrderPolicy: ValueOrderIterator,
	}
	base, err := BuildPackID("resource-a", result)
	if err != nil {
		t.Fatal(err)
	}
	otherResource, err := BuildPackID("resource-b", result)
	if err != nil {
		t.Fatal(err)
	}
	result.PackBytesDigest = bytes.Repeat([]byte{3}, 32)
	otherBytes, err := BuildPackID("resource-a", result)
	if err != nil {
		t.Fatal(err)
	}
	if base == otherResource {
		t.Fatal("resource scope did not affect pack id")
	}
	if base == otherBytes {
		t.Fatal("pack bytes digest did not affect pack id")
	}
}
