package packcopy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestDetectIndexFile checks each volume type's file is detected after a
// write, and a file of none is refused.
func TestDetectIndexFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	for _, kind := range []string{IndexSQLite, IndexBolt, IndexS4DB} {
		// Write a key, then reopen the file with its type detected.
		path := filepath.Join(dir, kind)
		store, closeStore, err := OpenIndexFile(ctx, path, &IndexFileOptions{Kind: kind, Table: "pack_index"})
		if err != nil {
			t.Fatal(err)
		}
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("k"), []byte("v")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := closeStore(); err != nil {
			t.Fatal(err)
		}
		if got, err := DetectIndexFile(path); err != nil || got != kind {
			t.Fatalf("detect %s = %q, %v", kind, got, err)
		}

		// The detected file reads the key back, read-only.
		store, closeStore, err = OpenIndexFile(ctx, path, &IndexFileOptions{Table: "pack_index", ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		rtx, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		v, found, err := rtx.Get(ctx, []byte("k"))
		rtx.Discard()
		if err != nil || !found || string(v) != "v" {
			t.Fatalf("%s read = %q, %v, %v", kind, v, found, err)
		}
		if err := closeStore(); err != nil {
			t.Fatal(err)
		}
	}

	// A file of no type, and an empty one, are refused.
	for name, data := range map[string]string{"text": "not a volume file at all", "empty": ""} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := OpenIndexFile(ctx, path, &IndexFileOptions{}); err == nil {
			t.Fatalf("opened the %s file", name)
		}
	}
}
