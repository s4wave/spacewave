package kvtx

import (
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
)

// putPath names a path that writes blocks into the volume, for its flows.
type putPath int

// The write paths, in the order GetFlows reports them.
const (
	// putDirect is PutBlock and PutBlockBatch.
	putDirect putPath = iota
	// putPublication is the atomic publications.
	putPublication
	// putOwned is PrepareOwnedBlock and PrepareOwnedBlockBatch, other than
	// cache fills.
	putOwned
	// putFill is the cache fills.
	putFill
	// putStaged is PrepareStagedBlock and PrepareStagedBlockBatch.
	putStaged
	// putRoots is the entries SetBucketRoots writes.
	putRoots
	// numPutPaths is the number of write paths.
	numPutPaths
)

// putPathNames are the names the flows report the write paths under.
var putPathNames = [numPutPaths]string{"direct", "publication", "owned", "fill", "staged", "roots"}

// putCount accumulates one write path's flow.
type putCount struct {
	// blocks and bytes are the flow's blocks and their length.
	blocks, bytes atomic.Uint64
	// unknown counts the flow's blocks stored without their references.
	unknown atomic.Uint64
}

// maxUnknownCallers bounds the callers each write path names for its puts
// without references. Later callers count under otherCaller.
const maxUnknownCallers = 8

// otherCaller names the callers past maxUnknownCallers.
const otherCaller = "other"

// callerFrames is the number of frames outside the volume a caller is named by.
const callerFrames = 4

// modulePrefix is trimmed from the function names a caller is named by.
const modulePrefix = "github.com/s4wave/spacewave/"

// plumbingPrefixes are the function name prefixes of the volume, the block
// collector and the block package, which forward puts rather than make them.
var plumbingPrefixes = []string{
	"runtime.",
	modulePrefix + "db/volume",
	modulePrefix + "db/block.",
	modulePrefix + "db/block/gc.",
}

// unknownCallers counts the puts without references by write path and caller.
type unknownCallers struct {
	mu sync.Mutex
	// puts counts the blocks by path, then caller.
	puts [numPutPaths]map[string]uint64
}

// ValueFamily is a key prefix of the volume's storage and the name its flows
// report the prefix under.
type ValueFamily struct {
	// Name names the keys, such as "block".
	Name string
	// Prefix is the prefix of the keys.
	Prefix []byte
}

// ValueFamilies returns the key families of a volume laid out by keys: its
// blocks, its object stores and its reference graph. A storage that counts
// its values by these reports the other keys as "other".
func ValueFamilies(keys *store_kvkey.KVKey) []ValueFamily {
	return []ValueFamily{
		{Name: "block", Prefix: keys.GetBlockFullPrefix()},
		{Name: "object", Prefix: keys.GetObjectStoreFullPrefix()},
		{Name: "graph", Prefix: refGraphPrefix},
	}
}

// SetValueFlows sets fn to report the values the volume's storage wrote and
// freed, by key family. It is called once, before the volume is used.
func (v *Volume) SetValueFlows(fn func() []volume.ValueFlow) {
	v.valueFlows = fn
}

// GetFlows returns the blocks each write path stored that the volume did not
// hold, and the values its storage wrote and freed, since the volume opened.
func (v *Volume) GetFlows() *volume.VolumeFlows {
	// Report each write path.
	flows := &volume.VolumeFlows{VolumeID: v.GetID()}
	for path := range numPutPaths {
		c := &v.puts[path]
		flows.Puts = append(flows.Puts, volume.PutFlow{
			Path:        putPathNames[path],
			Blocks:      c.blocks.Load(),
			Bytes:       c.bytes.Load(),
			RefsUnknown: c.unknown.Load(),
		})
	}

	// Report the callers of the puts without references.
	flows.UnknownCallers = v.unknownCallers.snapshot()

	// Report the storage's values when it counts them.
	if v.valueFlows != nil {
		flows.Values = v.valueFlows()
	}
	return flows
}

// countPut counts a block of n bytes that path stored, unless the volume held
// it already, and whether its references were known.
func (v *Volume) countPut(path putPath, n int, existed, refsKnown bool) {
	if !existed {
		v.puts[path].blocks.Add(1)
		v.puts[path].bytes.Add(uint64(n)) // #nosec G115 -- lengths are not negative.
		if !refsKnown {
			v.puts[path].unknown.Add(1)
		}
	}
}

// countPutBatch counts the entries path stored that the volume did not hold,
// as existed reports them. A tombstone stores no block.
func (v *Volume) countPutBatch(path putPath, entries []*block.PutBatchEntry, existed []bool) {
	// Sum the new blocks.
	var blocks, bytes, unknown uint64
	for i, e := range entries {
		if e.Tombstone || (i < len(existed) && existed[i]) {
			continue
		}
		blocks++
		bytes += uint64(len(e.Data))
		if !e.RefsKnown {
			unknown++
		}
	}

	// Count them.
	if blocks != 0 {
		v.puts[path].blocks.Add(blocks)
		v.puts[path].bytes.Add(bytes)
		v.puts[path].unknown.Add(unknown)
	}
}

// notePutRefs names the caller of a put through path when its writer did not
// declare the block's references. Call it from the caller's goroutine.
func (v *Volume) notePutRefs(path putPath, opts *block.PutOpts) {
	if !opts.GetRefsKnown() {
		v.unknownCallers.add(path, 1)
	}
}

// noteBatchRefs names the caller of a batch through path for each entry whose
// writer did not declare its references. Call it from the caller's goroutine.
func (v *Volume) noteBatchRefs(path putPath, entries []*block.PutBatchEntry) {
	var n uint64
	for _, e := range entries {
		if !e.Tombstone && !e.RefsKnown {
			n++
		}
	}
	if n != 0 {
		v.unknownCallers.add(path, n)
	}
}

// add counts n puts without references under path and the calling code.
func (u *unknownCallers) add(path putPath, n uint64) {
	// Name the caller before taking the lock.
	caller := callerName()

	// Count it, or count it as other once the path names enough callers.
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.puts[path] == nil {
		u.puts[path] = make(map[string]uint64)
	}
	if _, ok := u.puts[path][caller]; !ok && len(u.puts[path]) >= maxUnknownCallers {
		caller = otherCaller
	}
	u.puts[path][caller] += n
}

// snapshot returns the counted callers, by path order and then caller.
func (u *unknownCallers) snapshot() []volume.UnknownCaller {
	// Copy each path's callers in name order.
	u.mu.Lock()
	defer u.mu.Unlock()
	var out []volume.UnknownCaller
	for path := range numPutPaths {
		start := len(out)
		for caller, puts := range u.puts[path] {
			out = append(out, volume.UnknownCaller{Path: putPathNames[path], Caller: caller, Puts: puts})
		}
		slices.SortFunc(out[start:], func(a, b volume.UnknownCaller) int {
			return strings.Compare(a.Caller, b.Caller)
		})
	}
	return out
}

// callerName names the code that called into the volume by its innermost
// frames outside the volume, the block collector and the block package,
// joined innermost first.
func callerName() string {
	// Read the stack above this function.
	var pcs [32]uintptr
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs[:])])

	// Keep the first frames that are not plumbing.
	var names []string
	for len(names) < callerFrames {
		frame, more := frames.Next()
		if frame.Function != "" && !slices.ContainsFunc(plumbingPrefixes, func(p string) bool {
			return strings.HasPrefix(frame.Function, p)
		}) {
			names = append(names, strings.TrimPrefix(frame.Function, modulePrefix))
		}
		if !more {
			break
		}
	}
	if len(names) == 0 {
		return "unknown"
	}
	return strings.Join(names, " < ")
}

// _ is a type assertion
var _ volume.FlowReporter = (*Volume)(nil)
