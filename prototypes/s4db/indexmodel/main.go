// Command indexmodel estimates the bytes each index design writes per byte of
// index entries inserted, for random hash keys mixed with clustered object
// keys. It simulates page and run boundaries instead of storing data.
//
// Designs:
//   - cow: a copy-on-write B+tree that rewrites the root path every commit.
//   - ckpt: a B+tree whose commits go to a log, with dirty pages written once
//     per checkpoint of buffer bytes.
//   - leveled: an LSM with a buffer, four level-zero runs, and levels growing
//     tenfold, compacting one 2 MiB table at a time.
//   - tiered: an LSM that merges runs of similar size four at a time.
package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"slices"
)

// page is the index page size.
const page = 4096

// main prints the write amplification of each index design across key
// counts.
func main() {
	// Parse the entry and commit sizes.
	entry := flag.Int("entry", 56, "bytes per index entry")
	commit := flag.Int("commit", 128, "entries per commit")
	flag.Parse()

	// Print one row per key count.
	fmt.Printf("entry %d B, %d entries per commit; bytes written per entry byte (log included)\n", *entry, *commit)
	fmt.Printf("%10s %8s %8s %8s %8s %8s %8s %8s\n", "keys", "index", "cow", "ckpt4M", "ckpt32M", "ckpt128M", "leveled", "tiered")
	for _, n := range []int{100_000, 1_000_000, 4_000_000, 10_000_000} {
		keys := genKeys(n)
		in := float64(n * *entry)
		fmt.Printf("%10d %7.0fM %8.1f %8.1f %8.1f %8.1f %8.1f %8.1f\n", n, in/(1<<20),
			cow(keys, *entry, *commit)/in,
			ckpt(keys, *entry, 4<<20)/in,
			ckpt(keys, *entry, 32<<20)/in,
			ckpt(keys, *entry, 128<<20)/in,
			leveled(n, *entry, 4<<20)/in,
			tiered(n, *entry, 4<<20)/in)
	}
}

// genKeys returns n keys as sortable integers: seven in ten uniform random,
// the rest appended in order under a few thousand object prefixes.
func genKeys(n int) []uint64 {
	// Draw each key at random or from an object's next sequence.
	rng := rand.New(rand.NewPCG(1, 1))
	next := make([]uint64, 4096)
	keys := make([]uint64, n)
	for i := range keys {
		if i%10 < 7 {
			keys[i] = rng.Uint64()
			continue
		}
		obj := rng.IntN(len(next))
		keys[i] = uint64(obj)<<52 | 1<<51 | next[obj]
		next[obj]++
	}
	return keys
}

// tree tracks leaf boundaries and fill of a B+tree.
type tree struct {
	// lo holds the lowest key of each leaf, sorted.
	lo []uint64
	// fill holds the entry count of each leaf.
	fill []int
	// cap is the entries per full leaf.
	cap int
}

// newTree returns a tree with one empty leaf.
func newTree(entry int) *tree {
	return &tree{lo: []uint64{0}, fill: []int{0}, cap: page / entry}
}

// insert adds key and returns the leaf it landed in after any split.
func (t *tree) insert(key uint64) int {
	// Count the entry in the leaf covering key.
	i, _ := slices.BinarySearch(t.lo, key+1)
	i--
	t.fill[i]++
	if t.fill[i] <= t.cap {
		return i
	}

	// Split the full leaf in half; the new leaf's low key is approximated by
	// the inserted key, which keeps boundaries ordered.
	half := t.fill[i] / 2
	t.fill[i] -= half
	t.lo = slices.Insert(t.lo, i+1, key)
	t.fill = slices.Insert(t.fill, i+1, half)
	return i
}

// height returns the levels of the tree, with inner fanout 4096/24.
func (t *tree) height() int {
	h, n := 1, len(t.lo)
	for n > 1 {
		n = (n + page/24 - 1) / (page / 24)
		h++
	}
	return h
}

// pathPages returns the pages written to rewrite dirty leaves and their
// inner paths. Shared inner pages count once per level: the distinct inner
// pages are estimated as the dirty pages below divided by the fanout, at
// least one per level.
func (t *tree) pathPages(dirty int) int {
	pages := 0
	for range t.height() {
		pages += dirty
		dirty = max(1, (dirty+page/24-1)/(page/24))
	}
	return pages
}

// cow returns bytes written when each commit copies the leaf and inner path of
// every touched leaf.
func cow(keys []uint64, entry, commit int) float64 {
	// Write the dirty paths at the end of each commit.
	t := newTree(entry)
	var written float64
	dirty := map[int]bool{}
	for i, k := range keys {
		dirty[t.insert(k)] = true
		if (i+1)%commit == 0 {
			written += float64(t.pathPages(len(dirty)) * page)
			clear(dirty)
		}
	}
	return written
}

// ckpt returns bytes written when commits append entries to a log and a
// checkpoint writes each dirty page once per buf log bytes.
func ckpt(keys []uint64, entry, buf int) float64 {
	// Count the log, and write the dirty pages at each checkpoint.
	t := newTree(entry)
	written := float64(len(keys) * entry)
	dirty := map[int]bool{}
	per := buf / entry
	flush := func() {
		written += float64(t.pathPages(len(dirty)) * page)
		clear(dirty)
	}

	// Insert every key, checkpointing each buf log bytes.
	for i, k := range keys {
		// Leaf indexes shift on split; marking by low key would be exact, but
		// splits are rare relative to inserts and only shift the count.
		dirty[t.insert(k)] = true
		if (i+1)%per == 0 {
			flush()
		}
	}
	flush()
	return written
}

// leveled returns bytes written by a leveled LSM: the log, buffer flushes to
// level zero, level-zero merges into level one, and each deeper level
// receiving its parent's bytes with tenfold overlap.
func leveled(n, entry, buf int) float64 {
	// Count the log and the buffer flushes.
	total := float64(n * entry)
	written := 2 * total

	// Level one starts at four buffers and each level is ten times larger.
	// Every byte pushed into a level that already holds data rewrites about
	// ratio/2 bytes of overlap on average once the level is near its target.
	limit := float64(4 * buf)
	remaining := total
	for remaining > limit/10 {
		size := min(remaining, limit)
		// Bytes that pass through this level get merged with this level's
		// resident data; the average overlap factor grows with fill.
		fill := size / limit
		written += total * (1 + 10*fill/2)
		remaining -= size
		limit *= 10
	}
	return written
}

// tiered returns bytes written by a size-tiered LSM merging four runs at a
// time: each byte is rewritten once per tier.
func tiered(n, entry, buf int) float64 {
	total := float64(n * entry)
	written := 2 * total
	for run := float64(buf); run*4 <= total; run *= 4 {
		written += total
	}
	return written
}
