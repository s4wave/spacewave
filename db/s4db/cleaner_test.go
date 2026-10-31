package s4db

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestCleaner fills a file with large values, deletes many and rewrites some,
// then waits for the cleaner to bring the slack under its bound and the file
// end down, with every value intact and the flows unchanged by moves.
func TestCleaner(t *testing.T) {
	// Fill with 2,000 values of 2 to 30 KiB.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "clean.s4wave")
	opts := Options{CacheBytes: 8 << 20, memtable: 256 << 10}
	db := openTest(t, path, opts)
	r := rand.New(rand.NewPCG(7, 8))
	m := make(model)
	for i := range 2000 {
		if i%50 == 0 {
			tx, err := db.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			for j := i; j < i+50; j++ {
				k := fmt.Sprintf("v/%05d", j)
				v := randomBytes(r, 2048+r.IntN(28<<10))
				m[k] = v
				if err := tx.Set(ctx, []byte(k), v); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	before, _ := db.Stats(ctx)

	// Delete 40% in random order and rewrite a fifth.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	r.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	for i, k := range keys[:len(keys)*3/5] {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if i < len(keys)*2/5 {
			delete(m, k)
			err = tx.Delete(ctx, []byte(k))
		} else {
			v := randomBytes(r, 2048+r.IntN(8<<10))
			m[k] = v
			err = tx.Set(ctx, []byte(k), v)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	flowsBefore, _ := db.Stats(ctx)

	// Wait for the cleaner to bring the slack within an eighth of live.
	deadline := time.Now().Add(60 * time.Second)
	var s Stats
	for {
		var err error
		if s, err = db.Stats(ctx); err != nil {
			t.Fatal(err)
		}
		if s.DeadBytes+s.FreeBytes <= s.LiveBytes/cleanSlackDiv+chunkSize {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("slack stayed at dead %d free %d for live %d (file %d, before %d)", s.DeadBytes, s.FreeBytes, s.LiveBytes, s.FileBytes, before.FileBytes)
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.check(t, db)
	if s.FileBytes >= before.FileBytes {
		t.Fatalf("file did not shrink: %d, before %d", s.FileBytes, before.FileBytes)
	}
	if s.Families[0] != flowsBefore.Families[0] {
		t.Fatalf("moves changed the flows: %+v, before %+v", s.Families[0], flowsBefore.Families[0])
	}

	// Reopen and check again.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, opts)
	defer db.Close()
	m.check(t, db)
}

// raceKeys is the number of keys each race writer churns.
const raceKeys = 300

// raceValue returns the self-checking value of key i at version: its length
// and bytes follow from the key, and its first four bytes name the version.
func raceValue(i, version int) []byte {
	b := make([]byte, 3000+(i*7919)%5000)
	binary.LittleEndian.PutUint32(b, uint32(version)) // #nosec G115 -- a small test version.
	for j := 4; j < len(b); j++ {
		b[j] = byte(i*31 + version*17 + j)
	}
	return b
}

// raceCheck verifies a value against its key and embedded version.
func raceCheck(k, v []byte) error {
	// The key names the entry and the value its version.
	var who string
	var i int
	if _, err := fmt.Sscanf(string(k), "%1s/%04d", &who, &i); err != nil {
		return err
	}
	if len(v) < 4 || len(v) != 3000+(i*7919)%5000 {
		return fmt.Errorf("key %s: value length %d", k, len(v))
	}
	version := int(binary.LittleEndian.Uint32(v))
	if !bytes.Equal(v, raceValue(i, version)) {
		return fmt.Errorf("key %s: value does not match version %d", k, version)
	}
	return nil
}

// churn rewrites and deletes keys under prefix who with rising versions,
// from seed, until stop closes.
func churn(ctx context.Context, db *DB, who string, seed uint64, stop <-chan time.Time) error {
	r := rand.New(rand.NewPCG(seed, 1))
	for version := 1; ; version++ {
		select {
		case <-stop:
			return nil
		default:
		}
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		i := r.IntN(raceKeys)
		k := fmt.Appendf(nil, "%s/%04d", who, i)
		if r.IntN(5) == 0 {
			err = tx.Delete(ctx, k)
		} else {
			err = tx.Set(ctx, k, raceValue(i, version))
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			return err
		}
	}
}

// TestCleanerRace runs flushes, merges and the cleaner over a churning file
// under two writers in this process and one in a child process, each
// rewriting keys with self-checking values, and readers holding snapshots
// across many commits, and checks that every value read matches its key and
// version. It is the adversarial test for losing, exposing or inventing a
// value through a background job.
func TestCleanerRace(t *testing.T) {
	// Seed the file with large values.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "race.s4wave")
	opts := Options{CacheBytes: 8 << 20, memtable: 128 << 10}
	db := openTest(t, path, opts)
	defer db.Close()
	for i := range raceKeys {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, who := range []string{"a", "b", "c"} {
			if err := tx.Set(ctx, fmt.Appendf(nil, "%s/%04d", who, i), raceValue(i, 0)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Start the child churning its keys.
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), churnEnv+"="+path)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Writers churn their keys; readers check every key set they see.
	stop := make(chan time.Time)
	errs := make(chan error, 8)
	for w, who := range []string{"a", "b"} {
		go func() { errs <- churn(ctx, db, who, uint64(w), stop) }() // #nosec G115 -- a small index.
	}
	for range 4 {
		go func() {
			for {
				select {
				case <-stop:
					errs <- nil
					return
				default:
				}
				tx, err := db.NewTransaction(ctx, false)
				if err != nil {
					errs <- err
					return
				}
				for _, who := range []string{"a", "b", "c"} {
					if err == nil {
						err = tx.ScanPrefix(ctx, []byte(who+"/"), raceCheck)
					}
				}
				time.Sleep(2 * time.Millisecond)
				tx.Discard()
				if err != nil {
					errs <- err
					return
				}
			}
		}()
	}

	// Run for a while, then stop everyone and check the final contents.
	time.Sleep(4 * time.Second)
	close(stop)
	for range 6 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child: %v: %s", err, out.String())
	}
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for _, who := range []string{"a", "b", "c"} {
		if err := tx.ScanPrefix(ctx, []byte(who+"/"), raceCheck); err != nil {
			t.Fatal(err)
		}
	}
}

// randomBytes returns n random bytes from r.
func randomBytes(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	return b
}
