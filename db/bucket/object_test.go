package bucket

import (
	"testing"
)

func TestObjectRef(t *testing.T) {
	// Construct an object reference for a base58 round trip.
	r := &ObjectRef{
		BucketId: "test",
	}

	// Encode the object reference as a base58 string.
	rf := r.MarshalString()
	t.Log(rf)

	// Parse the encoded object reference.
	or, err := ParseObjectRef(rf)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the base58 round trip preserves the object reference.
	if !r.EqualVT(or) {
		t.Fail()
	}
}

func TestUnmarshalJSON(t *testing.T) {
	// Parse and validate an object reference with a JSON root field.
	dat := `{"rootRef": "2W1M3cypWDWXjwjZKPFVoPEZtHwTBo7xzU1YH1mAoVd2b8jHjy3r"}`
	ref, err := UnmarshalObjectRefJSON([]byte(dat))
	if err == nil {
		err = ref.Validate()
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Encode the validated object reference as JSON.
	out, err := ref.MarshalJSON()
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Log(string(out))

	// Parse the encoded object reference for comparison.
	outRef, err := UnmarshalObjectRefJSON(out)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the JSON round trip preserves the object reference.
	if !outRef.EqualVT(ref) {
		t.Fail()
	}
}
