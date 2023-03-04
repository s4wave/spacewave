package bucket

import (
	"testing"
)

func TestObjectRef(t *testing.T) {
	r := &ObjectRef{
		BucketId: "test",
	}
	rf := r.MarshalString()
	t.Log(rf)
	or, err := ParseObjectRef(rf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !r.EqualVT(or) {
		t.Fail()
	}
}

func TestUnmarshalJSON(t *testing.T) {
	dat := `{"rootRef": "2W1M3cypWDWXjwjZKPFVoPEZtHwTBo7xzU1YH1mAoVd2b8jHjy3r"}`
	ref, err := UnmarshalObjectRefJSON([]byte(dat))
	if err == nil {
		err = ref.Validate()
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	out, err := ref.MarshalJSON()
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Log(string(out))

	outRef, err := UnmarshalObjectRefJSON(out)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !outRef.EqualVT(ref) {
		t.Fail()
	}
}
