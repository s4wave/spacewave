package bucket

import (
	"github.com/golang/protobuf/proto"
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
	if !proto.Equal(r, or) {
		t.Fail()
	}
}
