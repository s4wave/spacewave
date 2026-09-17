//go:build !js && !wasip1

package kvtx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	db_kvtx "github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
)

type publicationCrashStore struct {
	db_kvtx.Store
	armed atomic.Bool
	after bool
}

func (*publicationCrashStore) SupportsAtomicCommit() bool { return true }
func (s *publicationCrashStore) NewTransaction(ctx context.Context, write bool) (db_kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &publicationCrashTx{Tx: tx, store: s}, nil
}

type publicationCrashTx struct {
	db_kvtx.Tx
	store *publicationCrashStore
}

func (t *publicationCrashTx) Commit(ctx context.Context) error {
	armed := t.store.armed.Load()
	if armed && !t.store.after {
		os.Exit(74)
	}
	err := t.Tx.Commit(ctx)
	if armed && err == nil {
		os.Exit(74)
	}
	return err
}

// Cut power to the process with both publications already applied to the raw
// transaction: before Commit, and after durable Commit but before any receipt.
func TestPublicationCrashAtomicGroup(t *testing.T) {
	const roleEnv = "SPACEWAVE_PUBLICATION_CRASH_ROLE"
	if role := os.Getenv(roleEnv); role != "" {
		publicationCrashWorker(t, role, os.Getenv("SPACEWAVE_PUBLICATION_CRASH_PATH"))
		return
	}
	for _, cut := range []string{"before", "after"} {
		t.Run(cut, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "publication.bolt")
			for _, step := range []struct {
				role string
				code int
			}{{"seed", 0}, {cut, 74}, {"verify-" + cut, 0}} {
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPublicationCrashAtomicGroup$", "-test.timeout=15s")
				cmd.Env = append(os.Environ(), roleEnv+"="+step.role, "SPACEWAVE_PUBLICATION_CRASH_PATH="+path)
				out, err := cmd.CombinedOutput()
				cancel()
				if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != step.code {
					t.Fatalf("%s: %v\n%s", step.role, err, out)
				}
			}
		})
	}
}

func publicationCrashWorker(t *testing.T, role, path string) {
	raw, err := store_bolt.Open(path, 0o600, nil, []byte("publication"))
	if err != nil {
		t.Fatal(err)
	}
	if raw.GetDB().NoSync || raw.GetDB().NoFreelistSync {
		t.Fatal("sync disabled")
	}
	s := &publicationCrashStore{Store: raw, after: role == "after"}
	v, err := NewVolume(t.Context(), "publication-crash", store_kvkey.NewDefaultKVKey(), s, &store_kvtx.Config{}, false, false, nil, raw.GetDB().Close)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	})
	zero := publicationFor(t, "crash", "", "zero")
	first := publicationFor(t, "crash", "zero", "one")
	second := publicationFor(t, "crash", "one", "two")
	if role == "seed" {
		if err := v.PublishAtomic(t.Context(), zero); err != nil {
			t.Fatal(err)
		}
		return
	}
	if strings.HasPrefix(role, "verify-") {
		want := "zero"
		found := role == "verify-after"
		if found {
			want = "two"
		}
		assertHead(t, v, "crash", want)
		assertPublishedBlock(t, v, zero, true)
		assertPublishedBlock(t, v, first, found)
		assertPublishedBlock(t, v, second, found)
		return
	}
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	first.Validate = func(context.Context, block.StoreOps) error { close(entered); <-release; return nil }
	r1 := submitPublication(t, v, first)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("no preflight")
	}
	second.After = r1
	r2 := submitPublication(t, v, second)
	s.armed.Store(true)
	// Both requests are admitted before the physical commit reaches its cut.
	close(release)
	released = true
	if err := awaitPublication(t, r2); err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash cut was not reached")
}
