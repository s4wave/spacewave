package volume_filesnap

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFileSnapStoreReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "snap.json")

	s1, err := NewStore(path)
	if err != nil {
		t.Fatal(err.Error())
	}
	tx1, err := s1.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := tx1.Set(ctx, []byte("a"), []byte("one")); err != nil {
		t.Fatal(err.Error())
	}
	if err := tx1.Set(ctx, []byte("b"), []byte("two")); err != nil {
		t.Fatal(err.Error())
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	tx1.Discard()

	s2, err := NewStore(path)
	if err != nil {
		t.Fatal(err.Error())
	}
	tx2, err := s2.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx2.Discard()
	for k, want := range map[string]string{"a": "one", "b": "two"} {
		got, found, err := tx2.Get(ctx, []byte(k))
		if err != nil || !found || string(got) != want {
			t.Fatalf("Get(%s) = %q %v %v; want %q", k, string(got), found, err, want)
		}
	}
}
