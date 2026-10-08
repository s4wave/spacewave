package main

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/keypem"
)

// TestStoreRepair exercises sidecar-free inspection and atomic physical readback.
func TestStoreRepair(t *testing.T) {
	// Seed a real volume with old state, history, queue, and unrelated records.
	state, key := legacyState(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "p_local_test.s4wave")
	db, err := bdb.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	base := "h/objs/p/local/test/so/"
	root := base + "so/" + testObjectID + "/"
	data, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	pem, err := keypem.MarshalPrivKeyPem(key)
	if err != nil {
		t.Fatal("fixture key encoding failed")
	}
	if err := db.Update(func(tx *bdb.Tx) error {
		// Persist the isolated fixture in the owning hydra bucket.
		bucket, err := tx.CreateBucketIfNotExists([]byte("hydra"))
		if err != nil {
			return err
		}
		for k, v := range map[string][]byte{
			"h/priv": pem, root + "host": data,
			root + "config-history/entry/old": {}, root + "config-history/checkpoint": {},
			root + "local/state":                                []byte("obsolete queue"),
			root + "ls/app/retained":                            []byte("retained application data"),
			"h/objs/p/spacewave/remote/account-state/unrelated": []byte("retained cloud cache"),
		} {
			if err := bucket.Put([]byte(k), v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Require every database and sidecar byte to remain unchanged after inspection.
	before := fileDigests(t, dir)
	keys := make(keySet)
	store, err := ReadStore(path, keys)
	if err != nil {
		t.Fatal(err)
	}
	after := fileDigests(t, dir)
	if len(before) != len(after) {
		t.Fatal("read-only inspection created a file")
	}
	for name, digest := range before {
		if after[name] != digest {
			t.Fatalf("read-only inspection changed %s", name)
		}
	}
	if len(store.records) != 1 {
		t.Fatal("host inventory did not isolate the local object")
	}

	// Commit and reopen the complete prescribed repair through the real store.
	record := store.records[0]
	owner, validator := record.signers(keys)
	record.next, record.genesis, err = MigrateState(record.id, record.state, owner, validator)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Write(); err != nil {
		t.Fatal(err)
	}

	// Check that the per-SO transaction preserved unrelated local and cloud records.
	reopened, err := openDatabase(path, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := reopened.View(func(tx *bdb.Tx) error {
		// Inspect durable records in the reopened physical snapshot.
		bucket := tx.Bucket([]byte("hydra"))
		if !bytes.Equal(bucket.Get([]byte(root+"ls/app/retained")), []byte("retained application data")) {
			t.Fatal("application data changed")
		}
		if !bytes.Equal(bucket.Get([]byte("h/objs/p/spacewave/remote/account-state/unrelated")), []byte("retained cloud cache")) {
			t.Fatal("cloud cache changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// fileDigests fingerprints an isolated fixture without printing its contents.
func fileDigests(t *testing.T, dir string) map[string][32]byte {
	// Read each small fixture file after all database handles have closed.
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string][32]byte, len(entries))
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[entry.Name()] = sha256.Sum256(data)
	}
	return out
}

// _ asserts the fixture uses the real generated state codec.
var _ interface{ MarshalVT() ([]byte, error) } = (*sobject.SOState)(nil)
