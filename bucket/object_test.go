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
