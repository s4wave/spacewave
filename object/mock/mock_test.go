package object_mock

import (
	"testing"

	"github.com/aperturerobotics/hydra/object"
)

func TestPrefixer(t *testing.T) {
	objs, tb := BuildTestStore(t)
	pf := object.NewPrefixer(objs, "test-prefix/")
	testSeq := "testing123"
	if err := pf.SetObject("test", []byte(testSeq)); err != nil {
		t.Fatal(err.Error())
	}
	val, found, err := pf.GetObject("test")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.FailNow()
	}
	if string(val) != testSeq {
		t.FailNow()
	}
	keys, err := pf.ListKeys("")
	if err != nil {
		t.Fatal(err.Error())
	}
	if keys[0] != "test" {
		t.Fatalf("expected test, got %s", keys[0])
	}
	if err := pf.DeleteObject("test"); err != nil {
		t.Fatal(err.Error())
	}
	_ = objs
	_ = tb
}
