//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
)

const (
	boltStoreChurnRoleEnv = "SPACEWAVE_BOLT_STORE_CHURN_ROLE"
	boltStoreChurnPathEnv = "SPACEWAVE_BOLT_STORE_CHURN_PATH"
	boltStoreChurnIDEnv   = "SPACEWAVE_BOLT_STORE_CHURN_ID"
)

func TestBoltStoreMultiprocessWriterChurn(t *testing.T) {
	if role := os.Getenv(boltStoreChurnRoleEnv); role != "" {
		runBoltStoreChurnRole(t, role)
		return
	}

	dbPath := filepath.Join(t.TempDir(), "store-churn.bolt")
	store := openBoltChurnStore(t, dbPath)
	tx, err := store.NewTransaction(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 128 {
		if err := tx.Set(context.Background(), fmt.Appendf(nil, "seed-%03d", i), boltStoreChurnValue(0, i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatal(err)
	}

	cmds := []*exec.Cmd{
		boltStoreChurnCommand(t, dbPath, 1),
		boltStoreChurnCommand(t, dbPath, 2),
	}
	for _, cmd := range cmds {
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("writer failed: %v\n%s", err, boltStoreChurnOutput(cmd))
		}
	}
}

func boltStoreChurnCommand(t *testing.T, dbPath string, id int) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBoltStoreMultiprocessWriterChurn$", "-test.v") //nolint:gosec
	cmd.Env = append(os.Environ(),
		boltStoreChurnRoleEnv+"=writer",
		boltStoreChurnPathEnv+"="+dbPath,
		boltStoreChurnIDEnv+"="+strconv.Itoa(id),
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	return cmd
}

func boltStoreChurnOutput(cmd *exec.Cmd) string {
	if buf, ok := cmd.Stdout.(*bytes.Buffer); ok {
		return buf.String()
	}
	return ""
}

func runBoltStoreChurnRole(t *testing.T, role string) {
	t.Helper()
	if role != "writer" {
		t.Fatalf("unknown bolt store churn role %q", role)
	}
	id, err := strconv.Atoi(os.Getenv(boltStoreChurnIDEnv))
	if err != nil {
		t.Fatal(err)
	}
	store := openBoltChurnStore(t, os.Getenv(boltStoreChurnPathEnv))
	defer store.db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for iter := range 80 {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := range 64 {
			key := fmt.Appendf(nil, "writer-%d-%03d-%03d", id, iter, i)
			if err := tx.Set(ctx, key, boltStoreChurnValue(iter, i)); err != nil {
				tx.Discard()
				t.Fatal(err)
			}
			if iter > 2 {
				if err := tx.Delete(ctx, fmt.Appendf(nil, "writer-%d-%03d-%03d", id, iter-3, i)); err != nil {
					tx.Discard()
					t.Fatal(err)
				}
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func openBoltChurnStore(t *testing.T, dbPath string) *Store {
	t.Helper()
	store, err := Open(dbPath, 0o644, &bdb.Options{
		Timeout:        10 * time.Second,
		NoFreelistSync: false,
		NoGrowSync:     false,
		FreelistType:   bdb.FreelistMapType,
		NoSync:         false,
	}, []byte("hydra"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func boltStoreChurnValue(iter, i int) []byte {
	value := make([]byte, 4096)
	for idx := range value {
		value[idx] = byte(iter + i + idx)
	}
	return value
}
