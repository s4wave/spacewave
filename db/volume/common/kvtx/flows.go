package kvtx

import (
	"errors"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
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
	// refused counts the path's puts refused for a new block without its
	// references.
	refused atomic.Uint64
	// callerLogged records that the first caller putting a block without its
	// references was logged.
	callerLogged atomic.Bool
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

// SetLogger sets the logger that reports the first caller of each write path
// putting a block without its references. It is called once, before the
// volume is used; without it nothing is logged.
func (v *Volume) SetLogger(le *logrus.Entry) {
	v.le = le
}

// SetValueFlows sets fn to report the values the volume's storage wrote and
// freed, by key family. It is called once, before the volume is used.
func (v *Volume) SetValueFlows(fn func() []volume.ValueFlow) {
	v.valueFlows = fn
}

// GetFlows returns the blocks each write path stored that the volume did not
// hold, the puts it refused, and the values its storage wrote and freed, since the volume opened.
func (v *Volume) GetFlows() *volume.VolumeFlows {
	// Report each write path.
	flows := &volume.VolumeFlows{VolumeID: v.GetID()}
	for path := range numPutPaths {
		c := &v.puts[path]
		flows.Puts = append(flows.Puts, volume.PutFlow{
			Path:    putPathNames[path],
			Blocks:  c.blocks.Load(),
			Bytes:   c.bytes.Load(),
			Refused: c.refused.Load(),
		})
	}

	// Report the storage's values when it counts them.
	if v.valueFlows != nil {
		flows.Values = v.valueFlows()
	}
	return flows
}

// countPut counts a block of n bytes that path stored, unless the volume held
// it already.
func (v *Volume) countPut(path putPath, n int, existed bool) {
	if existed {
		return
	}
	v.puts[path].blocks.Add(1)
	v.puts[path].bytes.Add(uint64(n)) // #nosec G115 -- lengths are not negative.
}

// countPutBatch counts the entries path stored that the volume did not hold,
// as existed reports them. A tombstone stores no block.
func (v *Volume) countPutBatch(path putPath, entries []*block.PutBatchEntry, existed []bool) {
	// Sum the new blocks.
	var blocks, bytes uint64
	for i, e := range entries {
		if e.Tombstone || (i < len(existed) && existed[i]) {
			continue
		}
		blocks++
		bytes += uint64(len(e.Data))
	}

	// Count them.
	if blocks != 0 {
		v.puts[path].blocks.Add(blocks)
		v.puts[path].bytes.Add(bytes)
	}
}

// countRefused counts a put on path that err refused for a new block without
// its references.
func (v *Volume) countRefused(path putPath, err error) {
	if errors.Is(err, block.ErrRefsUnknown) {
		v.puts[path].refused.Add(1)
	}
}

// noteUnknownRefs logs the caller of the first put on path of a block without
// its references, once per path for the life of the volume. It runs on the
// caller's goroutine, before the put, so the stack names the writer; the put
// may still find the block stored, which passes, or refuse it.
func (v *Volume) noteUnknownRefs(path putPath, data []byte, opts *block.PutOpts) {
	if len(data) != 0 && !opts.GetRefsKnown() {
		v.logUnknownRefsCaller(path)
	}
}

// noteUnknownRefsBatch logs the caller of the first batch on path holding a
// block without its references, as noteUnknownRefs does.
func (v *Volume) noteUnknownRefsBatch(path putPath, entries []*block.PutBatchEntry) {
	for _, e := range entries {
		if !e.Tombstone && !e.RefsKnown && len(e.Data) != 0 {
			v.logUnknownRefsCaller(path)
			return
		}
	}
}

// logUnknownRefsCaller logs the stack of the caller of a put on path without
// references, unless one was logged already.
func (v *Volume) logUnknownRefsCaller(path putPath) {
	if v.le == nil || !v.puts[path].callerLogged.CompareAndSwap(false, true) {
		return
	}
	v.le.WithField("path", putPathNames[path]).
		WithField("caller", callerStack(3)).
		Warn("block put without references")
}

// callerStack formats the stack above skip frames, innermost first, as
// "function file:line" separated by " < ".
func callerStack(skip int) string {
	var pcs [24]uintptr
	frames := runtime.CallersFrames(pcs[:runtime.Callers(skip+1, pcs[:])])
	var b strings.Builder
	for {
		f, more := frames.Next()
		if b.Len() != 0 {
			b.WriteString(" < ")
		}
		b.WriteString(f.Function)
		b.WriteByte(' ')
		b.WriteString(f.File)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(f.Line))
		if !more {
			return b.String()
		}
	}
}

// _ is a type assertion
var _ volume.FlowReporter = (*Volume)(nil)
