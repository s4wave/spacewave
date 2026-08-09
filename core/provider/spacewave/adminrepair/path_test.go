package adminrepair

import (
	"testing"
)

func TestPath(t *testing.T) {
	got := Path("01kny7hn4wp25f7t86xzww6bd6")
	want := "/api/admin/bstore/01kny7hn4wp25f7t86xzww6bd6/pack-metadata-repair"
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
