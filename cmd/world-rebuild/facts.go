package main

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
)

// subKey is one key name the write-back object store writes under its id,
// with the shape of the key after the name.
type subKey struct {
	name string
	// prefix is set for a name that prefixes keys; otherwise the key is the
	// name exactly.
	prefix bool
	// slashes is the number of '/' after a prefix name, or -1 for any.
	slashes int
}

// subKeys are the key names of db/block/store/writeback, relative to its
// object store:
//
//	marked/<hash>                 queue.go:21 markedPrefix
//	queue/<16 hex>/<hash>         queue.go:27 queuePrefix
//	evictable/<16 hex>/<hash>     evict.go:20 evictPrefix
//	evictable-bytes               evict.go:26 evictableBytesKey
//	epoch                         evict.go:23 epochKey
//	queue-count                   pending.go:42 countKey
//	reads/<16 hex>/<16 hex>       reads.go:19 readsPrefix, reads.go:73-75
//	copy/<target>                 copy.go:22 copyCursorPrefix
//	copy-done/<target>            copy.go:25 copyDonePrefix
//	backfill                      writeback.go:36 backfillKey
//	pending/, target, queue-pending  writeback.go:41 formerPrefixes (legacy)
var subKeys = []subKey{
	{markedSeg, true, 0},
	{"queue/", true, 1},
	{evictSeg, true, 1},
	{readsSeg, true, 1},
	{"copy/", true, -1},
	{"copy-done/", true, -1},
	{"pending/", true, -1},
	{evictBytesSeg, false, 0},
	{"epoch", false, 0},
	{"queue-count", false, 0},
	{"backfill", false, 0},
	{"target", false, 0},
	{"queue-pending", false, 0},
}

// isSubKey reports whether rest is a key the write-back store writes.
func isSubKey(rest string) bool {
	for _, k := range subKeys {
		if !k.prefix {
			if rest == k.name {
				return true
			}
			continue
		}
		if !strings.HasPrefix(rest, k.name) {
			continue
		}
		r := rest[len(k.name):]
		if r == "" || strings.HasPrefix(r, "/") || strings.HasSuffix(r, "/") {
			continue
		}
		if k.slashes < 0 || strings.Count(r, "/") == k.slashes {
			return true
		}
	}
	return false
}

// Object store key split results.
const (
	splitOK = iota
	splitNotObj
	splitUnknown
	splitAmbiguous
	// splitForeign is a key of an object store that is not a write-back
	// store: its id has no writeback segment. It holds no drop fact.
	splitForeign
)

// wbSeg is the last segment of every write-back object store id, as
// core/provider/local/id.go BlockStoreWritebackObjectStoreID builds it.
const wbSeg = "writeback"

// splitObjKey splits an object store key into its store id and the key
// inside it. Write-back store ids hold slashes (p/<provider>/<account>/blk/
// <store>/writeback), so the split is after the key's first writeback
// segment, and the rest must be a key the write-back store writes: a store
// id never holds a writeback segment but its last. A key with no writeback
// segment is splitForeign. A rest that is no such key is splitUnknown, and
// a later writeback segment that also leaves one is splitAmbiguous. Nothing
// is guessed.
func splitObjKey(key []byte) (string, string, int) {
	if !bytes.HasPrefix(key, []byte(objsPrefix)) {
		return "", "", splitNotObj
	}
	s := string(key[len(objsPrefix):])
	first, n := -1, 0
	for i := len(wbSeg); i <= len(s); i++ {
		if (i < len(s) && s[i] != '/') || !isWritebackID(s[:i]) {
			continue
		}
		if first < 0 {
			first = i
		}
		if i < len(s) && isSubKey(s[i+1:]) {
			n++
		}
	}
	switch {
	case first < 0:
		return "", "", splitForeign
	case first == len(s) || !isSubKey(s[first+1:]):
		return "", "", splitUnknown
	case n > 1:
		return "", "", splitAmbiguous
	}
	return s[:first], s[first+1:], splitOK
}

// isWritebackID reports whether id is a write-back object store id: its last
// segment is writeback.
func isWritebackID(id string) bool {
	return id == wbSeg || strings.HasSuffix(id, "/"+wbSeg)
}

// prodAccount is the provider account of production's write-back store:
// its id is p/local/hyperline-backend/blk/<store>/writeback.
const prodAccount = "hyperline-backend"

// writebackID is the write-back object store id the drop rule takes facts
// from: -writeback-id, or production's for the block store, as
// BlockStoreWritebackObjectStoreID builds it.
func (o opts) writebackID() string {
	if o.wbID != "" {
		return o.wbID
	}
	return provider_local.BlockStoreWritebackObjectStoreID(provider_local.ProviderID, prodAccount, o.storeID)
}

// idErr refuses a -writeback-id that is not the write-back store of the
// -store-id block store: production names both by one id, the write-back
// store's as BlockStoreWritebackObjectStoreID builds it from the provider,
// account and block store.
func (o opts) idErr() error {
	if o.wbID == "" {
		return nil
	}
	head, ok := strings.CutSuffix(o.wbID, "/blk/"+o.storeID+"/writeback")
	if rest, okp := strings.CutPrefix(head, "p/"); ok && okp {
		if provider, account, oks := strings.Cut(rest, "/"); oks && provider != "" && account != "" &&
			provider_local.BlockStoreWritebackObjectStoreID(provider, account, o.storeID) == o.wbID {
			return nil
		}
	}
	return fmt.Errorf("-writeback-id %q is not the write-back store of block store %q (want p/<provider>/<account>/blk/%s/writeback); refusing", o.wbID, o.storeID, o.storeID)
}

// ownStore reports whether obj is the write-back object store of the block
// store rebuilt: its id exactly. Any other write-back store, of another
// provider, account or block store, is another store's.
func ownStore(o opts, obj string) bool {
	return obj == o.writebackID()
}

// otherFactsErr refuses a World where another write-back store holds a
// marker or an eviction entry: the block table is shared by every store of
// the volume, so the drop rule of one store cannot decide such a block.
func otherFactsErr(f *facts) error {
	if len(f.otherFacts) == 0 {
		return nil
	}
	return fmt.Errorf("write-back stores other than the one rebuilt hold marked/ or evictable/ rows (%v); refusing", f.otherFacts)
}

// noteCopy records a copy cursor or completion row.
func (f *facts) noteCopy(k objKeyInfo) {
	if f.copies == nil {
		f.copies = map[[2]string]bool{}
	}
	id := [2]string{k.obj, k.gen}
	f.copies[id] = f.copies[id] || k.kind == kindCopyDone
}

// unfinishedCopiesErr refuses a World where a write-back copy to a remote
// is under way: a copy/<target> row with no copy-done/<target>. Its blocks
// may not be on the remote yet.
func unfinishedCopiesErr(f *facts) error {
	var open []string
	for id, done := range f.copies {
		if !done {
			open = append(open, id[0]+" copy/"+id[1])
		}
	}
	if len(open) == 0 {
		return nil
	}
	sort.Strings(open)
	return fmt.Errorf("%d write-back copies to a remote are unfinished (copy/ with no copy-done/): %v; refusing", len(open), open)
}

// objKey splits an object store key into its id and the key inside it; ok is
// false for a key outside the object stores or one splitObjKey refuses.
func objKey(key []byte) (string, string, bool) {
	obj, rest, st := splitObjKey(key)
	return obj, rest, st == splitOK
}

// evictSize parses an eviction entry's value, the block size.
func evictSize(v []byte) (int64, bool) {
	n, err := strconv.ParseInt(string(v), 10, 64)
	return n, err == nil && n >= 0
}

// Object store key kinds.
const (
	kindOther = iota
	kindBad
	kindMarked
	kindEvict
	kindReads
	kindBytes
	kindQueue
	// kindForeign is a key of an object store that is not a write-back
	// store; it is kept and holds no drop fact.
	kindForeign
	// kindCopy and kindCopyDone are a copy to a remote's cursor and its
	// completion mark; gen holds the target.
	kindCopy
	kindCopyDone
)

// objKeyInfo is one object store key, split: its store and kind, with the
// target of a copy row. It decides nothing: the rebuilt store's keys are
// decoded by facts.addKey with the store's own functions.
type objKeyInfo struct {
	ok   bool
	obj  string
	kind int
	gen  string
}

// objKeyOf splits an object store key into its store and kind.
func objKeyOf(key []byte) objKeyInfo {
	obj, rest, st := splitObjKey(key)
	switch st {
	case splitNotObj:
		return objKeyInfo{}
	case splitUnknown, splitAmbiguous:
		return objKeyInfo{ok: true, kind: kindBad}
	case splitForeign:
		return objKeyInfo{ok: true, kind: kindForeign}
	}
	k := objKeyInfo{ok: true, obj: obj}
	switch {
	case strings.HasPrefix(rest, markedSeg):
		k.kind = kindMarked
	case strings.HasPrefix(rest, evictSeg):
		k.kind = kindEvict
	case strings.HasPrefix(rest, readsSeg):
		k.kind = kindReads
	case rest == evictBytesSeg:
		k.kind = kindBytes
	case strings.HasPrefix(rest, "queue/"):
		k.kind = kindQueue
	case strings.HasPrefix(rest, "copy/"):
		k.kind, k.gen = kindCopy, rest[len("copy/"):]
	case strings.HasPrefix(rest, "copy-done/"):
		k.kind, k.gen = kindCopyDone, rest[len("copy-done/"):]
	}
	return k
}
