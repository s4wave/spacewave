//go:build linux

package kvtx

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"golang.org/x/sys/unix"
)

// Temporary measurement of the adoption recount on a cold page cache; not
// committed. RECOUNT_BLOCKS sets the stored block count.
func TestRecountMeasure(t *testing.T) {
	n, err := strconv.Atoi(os.Getenv("RECOUNT_BLOCKS"))
	if err != nil || n <= 0 {
		t.Skip("RECOUNT_BLOCKS unset")
	}
	ctx := t.Context()
	dir := t.TempDir()
	path := filepath.Join(dir, "volume.s4wave")
	keys := store_kvkey.NewDefaultKVKey()
	conf := &store_kvtx.Config{}

	// Fill: trees of 98 unique leaves and 2 shared from a pool of 1000, so
	// some blocks have several holders, each tree a named root.
	const fan = 100
	roots := n / (fan + 1)
	v, db := openRecountVolume(t, path, keys, conf)
	start := time.Now()
	for i := range roots {
		ents, pr := recountTree(t, i, fan)
		if err := v.SetBucketRoots(ctx, "fill", ents, []block.NamedRoot{{Name: "r" + strconv.Itoa(i), Ref: pr}}); err != nil {
			t.Fatal(err)
		}
	}
	markUnadopted(t, v)
	closeRecountVolume(t, v, db)
	st, _ := os.Stat(path)
	t.Logf("filled %d roots of %d blocks in %v, file %d MiB", roots, fan+1, time.Since(start), st.Size()>>20)

	// Evict the file from the page cache, then reopen.
	evictDir(t, dir)
	v, db = openRecountVolume(t, path, keys, conf)
	defer closeRecountVolume(t, v, db)

	// A writer commits a small tree every 20 ms beside the recount.
	var mu sync.Mutex
	var lat []time.Duration
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		prev := make([]*block.BlockRef, 50)
		pace := time.NewTicker(20 * time.Millisecond)
		defer pace.Stop()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			case <-pace.C:
			}
			ents, pr := recountWriterTree(t, i)
			k := i % 50
			t0 := time.Now()
			if err := v.SetBucketRoots(ctx, "w", ents, []block.NamedRoot{{Name: "w" + strconv.Itoa(k), Ref: pr, Previous: prev[k]}}); err != nil {
				t.Error(err)
				return
			}
			d := time.Since(t0)
			prev[k] = pr
			mu.Lock()
			lat = append(lat, d)
			mu.Unlock()
		}
	})

	// Recount, reading the process's I/O around it.
	io0 := readProcIO(t)
	start = time.Now()
	res, err := v.Adopt(ctx)
	dur := time.Since(start)
	io1 := readProcIO(t)
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("recount %v: blocks=%d roots=%d counted=%d queued=%d unknown=%d", dur, res.Blocks, res.Roots, res.Counted, res.Queued, res.Unknown)
	t.Logf("recount io: read syscalls=%d read_bytes=%d MiB rchar=%d MiB", io1["syscr"]-io0["syscr"], (io1["read_bytes"]-io0["read_bytes"])>>20, (io1["rchar"]-io0["rchar"])>>20)
	slices.Sort(lat)
	if len(lat) != 0 {
		t.Logf("writer beside recount: commits=%d p50=%v p99=%v max=%v", len(lat), lat[len(lat)/2], lat[len(lat)*99/100], lat[len(lat)-1])
	}
}

// openRecountVolume opens the s4db file at path as a Volume with frees off,
// so no background adoption runs.
func openRecountVolume(t *testing.T, path string, keys *store_kvkey.KVKey, conf *store_kvtx.Config) (*Volume, *s4db.DB) {
	t.Helper()
	families := ValueFamilies(keys)
	prefixes := make([][]byte, len(families))
	for i, f := range families {
		prefixes[i] = f.Prefix
	}
	db, err := s4db.Open(path, s4db.Options{Families: prefixes})
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewVolume(t.Context(), "test/recount", keys, db, conf, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return v, db
}

// closeRecountVolume closes v and its database.
func closeRecountVolume(t *testing.T, v *Volume, db *s4db.DB) {
	t.Helper()
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// recountTree builds tree i: 98 unique leaves and 2 from a shared pool under
// one parent.
func recountTree(t *testing.T, i, fan int) ([]*block.PutBatchEntry, *block.BlockRef) {
	var ents []*block.PutBatchEntry
	var refs []*block.BlockRef
	for j := range fan {
		tag := "leaf " + strconv.Itoa(i) + " " + strconv.Itoa(j)
		if j >= fan-2 {
			tag = "shared " + strconv.Itoa((i*2+j)%1000)
		}
		d := []byte(tag + " padding padding padding padding padding")
		r, err := block.BuildBlockRef(d, nil)
		if err != nil {
			t.Fatal(err)
		}
		ents = append(ents, &block.PutBatchEntry{Ref: r, Data: d, RefsKnown: true})
		refs = append(refs, r)
	}
	p := []byte("parent " + strconv.Itoa(i))
	for _, r := range refs {
		p = append(p, r.MarshalString()...)
	}
	pr, err := block.BuildBlockRef(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	return append(ents, &block.PutBatchEntry{Ref: pr, Data: p, Refs: refs, RefsKnown: true}), pr
}

// recountWriterTree builds the writer's tree i: three leaves under a parent.
func recountWriterTree(t *testing.T, i int) ([]*block.PutBatchEntry, *block.BlockRef) {
	var ents []*block.PutBatchEntry
	var refs []*block.BlockRef
	for j := range 3 {
		d := []byte("writer " + strconv.Itoa(i) + " " + strconv.Itoa(j))
		r, err := block.BuildBlockRef(d, nil)
		if err != nil {
			t.Fatal(err)
		}
		ents = append(ents, &block.PutBatchEntry{Ref: r, Data: d, RefsKnown: true})
		refs = append(refs, r)
	}
	p := []byte("writer parent " + strconv.Itoa(i))
	for _, r := range refs {
		p = append(p, r.MarshalString()...)
	}
	pr, err := block.BuildBlockRef(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	return append(ents, &block.PutBatchEntry{Ref: pr, Data: p, Refs: refs, RefsKnown: true}), pr
}

// evictDir drops every file in dir from the page cache.
func evictDir(t *testing.T, dir string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Sync(); err != nil {
			t.Log(err)
		}
		if err := unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}

// readProcIO reads /proc/self/io.
func readProcIO(t *testing.T) map[string]int64 {
	t.Helper()
	b, err := os.ReadFile("/proc/self/io")
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]int64)
	for line := range strings.SplitSeq(string(b), "\n") {
		k, val, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		n, _ := strconv.ParseInt(val, 10, 64)
		out[k] = n
	}
	return out
}
