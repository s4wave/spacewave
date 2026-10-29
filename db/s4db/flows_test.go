//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"testing"
)

// TestFlowsByFamily checks that the flows count the values commits store
// outside the index, and the stored values they replace or delete, under each
// key's family, and that they balance to the live value bytes.
func TestFlowsByFamily(t *testing.T) {
	// Open a database counting two families.
	ctx := context.Background()
	families := [][]byte{[]byte("a/"), []byte("b/")}
	db := openTest(t, filepath.Join(t.TempDir(), "flows.s4wave"), Options{Families: families})
	defer db.Close()
	commit := func(set map[string][]byte, del ...string) {
		// Set and delete the keys in one transaction.
		t.Helper()
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range set {
			if err := tx.Set(ctx, []byte(k), v); err != nil {
				t.Fatal(err)
			}
		}
		for _, k := range del {
			if err := tx.Delete(ctx, []byte(k)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	check := func(what string, want []Flow) {
		t.Helper()
		if got := db.Flows(); !slices.Equal(got, want) {
			t.Fatalf("%s: flows %v, want %v", what, got, want)
		}
	}

	// Store a large value in each family and outside them, and a small one
	// the index holds.
	big, bigger := bytes.Repeat([]byte{1}, 4096), bytes.Repeat([]byte{2}, 8192)
	commit(map[string][]byte{"a/1": big, "b/1": big, "c/1": big, "a/small": []byte("inline")})
	check("first commit", []Flow{{Written: 4096}, {Written: 4096}, {Written: 4096}})

	// Replace a's value and delete b's.
	commit(map[string][]byte{"a/1": bigger}, "b/1")
	check("second commit", []Flow{{Written: 4096 + 8192, Freed: 4096}, {Written: 4096, Freed: 4096}, {Written: 4096}})

	// The flows balance to the live value bytes.
	sizes, err := db.Sizes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var live int64
	for _, f := range db.Flows() {
		live += f.Written - f.Freed
	}
	if live != sizes.ValueBytes {
		t.Fatalf("flows balance to %d, live value bytes %d", live, sizes.ValueBytes)
	}
}
