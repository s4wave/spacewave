package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// kindNames names the object store key kinds for the census.
var kindNames = map[int]string{
	kindOther:   "other",
	kindBad:     "BAD",
	kindMarked:  "marked",
	kindEvict:   "evictable",
	kindReads:   "reads",
	kindBytes:   "evictable-bytes",
	kindQueue:   "queue",
	kindForeign: "other: not write-back",
}

// censusKey is one key shape: its pattern, object store id and kind.
type censusKey struct{ shape, obj, kind string }

// census counts the object store keys by shape.
type census struct {
	shapes map[censusKey]*tally
	// queued counts the queue/ entries (pending upload) and the sizes
	// their values record.
	queuedKeys, queuedBytes int64
	// evictBytes sums the sizes the evictable/ values record.
	evictBytes int64
}

// keyShape returns a pattern for the key inside an object store: the
// sub-key name with each later segment generalized.
func keyShape(rest string) string {
	segs := strings.Split(rest, "/")
	for i := 1; i < len(segs); i++ {
		segs[i] = "<" + strconv.Itoa(len(segs[i])) + ">"
	}
	return strings.Join(segs, "/")
}

// badShape returns a pattern for an object store key that did not split:
// segments longer than 24 bytes are generalized.
func badShape(key []byte) string {
	segs := strings.Split(string(key[len(objsPrefix):]), "/")
	for i, s := range segs {
		if len(s) > 24 {
			segs[i] = "<" + strconv.Itoa(len(s)) + ">"
		}
	}
	return objsPrefix + strings.Join(segs, "/")
}

// add counts one object store key with its value bytes and value.
func (c *census) add(key []byte, k objKeyInfo, vlen int64, value []byte) {
	// Make the shape table on first use.
	if c.shapes == nil {
		c.shapes = map[censusKey]*tally{}
	}

	// Name the key's shape: a bad key by its raw path.
	ck := censusKey{kind: kindNames[k.kind], obj: k.obj}
	if k.kind == kindBad || k.kind == kindForeign {
		ck.shape = badShape(key)
		ck.obj = "?"
	} else {
		_, rest, _ := objKey(key)
		ck.shape = keyShape(rest)
	}

	// Count the row.
	t := c.shapes[ck]
	if t == nil {
		t = &tally{}
		c.shapes[ck] = t
	}
	t.keys++
	t.bytes += vlen

	// Sum the sizes the queue and eviction values record.
	switch k.kind {
	case kindQueue:
		c.queuedKeys++
		if int64(len(value)) == vlen {
			s, _, _ := strings.Cut(string(value), "@")
			n, _ := strconv.ParseInt(s, 10, 64)
			c.queuedBytes += n
		}
	case kindEvict:
		if int64(len(value)) == vlen {
			n, _ := evictSize(value)
			c.evictBytes += n
		}
	}
}

// bad returns the rows the census could not classify, and those classed
// other.
func (c *census) bad() (bad, other int64) {
	for k, t := range c.shapes {
		switch k.kind {
		case "BAD":
			bad += t.keys
		case "other", kindNames[kindForeign]:
			other += t.keys
		}
	}
	return bad, other
}

// print reports the census.
func (c *census) print() {
	// Order the shapes by store, then shape.
	keys := make([]censusKey, 0, len(c.shapes))
	for k := range c.shapes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.obj != b.obj {
			return a.obj < b.obj
		}
		return a.shape < b.shape
	})

	// Print each shape.
	fmt.Println("\nobject store key census (shape, store id, kind: rows, value bytes):")
	for _, k := range keys {
		t := c.shapes[k]
		fmt.Printf("  %-40s %-50s %-16s %12d rows %14d value bytes\n", k.shape, k.obj, k.kind, t.keys, t.bytes)
	}

	// Print the rows not classed as drop facts, and their shapes.
	bad, other := c.bad()
	fmt.Printf("object store rows classed BAD (unclassifiable): %d\n", bad)
	fmt.Printf("object store rows classed other (kept, no drop fact): %d\n", other)
	for _, k := range keys {
		if k.kind == "BAD" || k.kind == "other" || k.kind == kindNames[kindForeign] {
			fmt.Printf("  %s shape: %s (%s) %d rows\n", k.kind, k.shape, k.obj, c.shapes[k].keys)
		}
	}

	// Print the totals to compare with the backend metrics.
	fmt.Printf("queued for upload (queue/ rows): %d blocks, %d bytes\n", c.queuedKeys, c.queuedBytes)
	fmt.Printf("total evictable bytes (sum of evictable/ values): %d\n", c.evictBytes)
}
