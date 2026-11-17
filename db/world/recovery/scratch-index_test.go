//go:build !js && !wasip1

package world_recovery

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
)

// TestScratchBudget checks every pooled connection and transactional read-error propagation.
func TestScratchBudget(t *testing.T) {
	// Hold four connections at once so each independently applies the configured budget.
	index, err := openScratch(filepath.Join(t.TempDir(), "scratch.sqlite"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	var connections []*sql.Conn
	for range 4 {
		connection, err := index.store.GetDB().Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
		for _, pragma := range []struct {
			name string
			want int64
		}{{"mmap_size", 0}, {"cache_size", -2048}, {"temp_store", 1}} {
			var value int64
			if err := connection.QueryRowContext(t.Context(), "PRAGMA "+pragma.name).Scan(&value); err != nil {
				t.Fatal(err)
			}
			if value != pragma.want {
				t.Fatalf("connection %d %s=%d, want %d", len(connections), pragma.name, value, pragma.want)
			}
		}
	}
	for _, connection := range connections {
		if err := connection.Close(); err != nil {
			t.Fatal(err)
		}
	}

	// An absent row is distinct from a failed SQL lookup even if a callback ignores it.
	if err := index.Update(func(tx *scratchTx) error { return tx.Bucket([]byte("test")).Put([]byte("key"), []byte("value")) }); err != nil {
		t.Fatal(err)
	}
	if _, err := index.store.GetDB().ExecContext(context.Background(), "DROP TABLE recovery"); err != nil {
		t.Fatal(err)
	}
	if err := index.Update(func(tx *scratchTx) error { _ = tx.Bucket([]byte("test")).Get([]byte("key")); return nil }); err == nil {
		t.Fatal("indexed read failure committed as an absent row")
	}
}

// TestRecoveryTransformBounds rejects expanding decoders before any native reader can invoke them.
func TestRecoveryTransformBounds(t *testing.T) {
	for _, id := range []string{"hydra/transform/snappy", "hydra/transform/lz4"} {
		ref := &bucket.ObjectRef{TransformConf: &block_transform.Config{Steps: []*block_transform.StepConfig{{Id: id}}}}
		if err := validateContext(ref); err == nil {
			t.Fatalf("unbounded transform %s admitted", id)
		}
	}
	if err := validateContext(&bucket.ObjectRef{}); err != nil {
		t.Fatal(err)
	}
}
