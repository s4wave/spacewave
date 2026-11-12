package block

import (
	"context"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
)

// Scratch hole check: refuse to store a block whose child is neither in the
// same batch, kept here, nor held by the inner store.

var (
	holeCalls, holeLookups, holeMax atomic.Int64
	sweptMtx                        sync.Mutex
	sweptAt                         = make(map[string]string)
)

// HoleCheckStats returns the checks run, the inner existence lookups they
// made, and the most lookups one check made.
func HoleCheckStats() (calls, lookups, most int64) {
	return holeCalls.Load(), holeLookups.Load(), holeMax.Load()
}

// recordSwept remembers the stack that swept key.
func recordSwept(key string) {
	sweptMtx.Lock()
	sweptAt[key] = string(debug.Stack())
	sweptMtx.Unlock()
}

// checkHoles panics if a block in batch names a child that is neither in
// batch, kept, nor in the inner store.
func (s *PendingStore) checkHoles(ctx context.Context, where string, batch []*PutBatchEntry) {
	holeCalls.Add(1)
	in := make(map[string]struct{}, len(batch))
	for _, e := range batch {
		in[pendingKey(e.Ref)] = struct{}{}
	}
	var refs []*BlockRef
	var parents []*BlockRef
	s.mtx.RLock()
	for _, e := range batch {
		for _, ref := range e.Refs {
			if ref.GetEmpty() {
				continue
			}
			key := pendingKey(ref)
			if _, ok := in[key]; ok {
				continue
			}
			if s.kept[key] != nil {
				continue
			}
			refs = append(refs, ref)
			parents = append(parents, e.Ref)
		}
	}
	s.mtx.RUnlock()
	n := int64(len(refs))
	holeLookups.Add(n)
	for {
		most := holeMax.Load()
		if n <= most || holeMax.CompareAndSwap(most, n) {
			break
		}
	}
	if len(refs) == 0 {
		return
	}
	found, err := s.StoreOps.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return
	}
	for i, ok := range found {
		if ok {
			continue
		}
		key := pendingKey(refs[i])
		sweptMtx.Lock()
		swept := sweptAt[key]
		sweptMtx.Unlock()
		panic(fmt.Sprintf(
			"HOLE: %s stores %s naming %s, which is neither kept nor stored\nswept by:\n%s",
			where, parents[i].MarshalString(), refs[i].MarshalString(), swept,
		))
	}
}

var putAt, freedAt = make(map[string]string), make(map[string]string)

// RecordPut remembers the stack of a put past the bound to dst.
func RecordPut(how string, ref *BlockRef) {
	sweptMtx.Lock()
	putAt[pendingKey(ref)] = how + "\n" + string(debug.Stack())
	events = append(events, "put "+how+" "+ref.MarshalString())
	sweptMtx.Unlock()
}

// RecordFree remembers the stack of the collector free of ref.
func RecordFree(ref *BlockRef) {
	sweptMtx.Lock()
	freedAt[pendingKey(ref)] = string(debug.Stack())
	events = append(events, "free "+ref.MarshalString())
	sweptMtx.Unlock()
}

// Stacks returns the put, free and sweep stacks of the b58 ref.
func Stacks(ref string) (put, free, swept string) {
	br, err := UnmarshalBlockRefB58(ref)
	if err != nil {
		return "bad ref: " + err.Error(), "", ""
	}
	k := pendingKey(br)
	sweptMtx.Lock()
	defer sweptMtx.Unlock()
	return putAt[k], freedAt[k], sweptAt[k]
}

// SweptStack returns the stack that swept the block named by the b58 ref, if
// a Sweep dropped it.
func SweptStack(ref string) string {
	br, err := UnmarshalBlockRefB58(ref)
	if err != nil {
		return "bad ref: " + err.Error()
	}
	sweptMtx.Lock()
	defer sweptMtx.Unlock()
	return sweptAt[pendingKey(br)]
}

var events []string

// Event appends an ordered trace line.
func Event(line string) {
	sweptMtx.Lock()
	events = append(events, line)
	sweptMtx.Unlock()
}

// Events returns the trace.
func Events() []string {
	sweptMtx.Lock()
	defer sweptMtx.Unlock()
	return slices.Clone(events)
}

// checkRootHoles panics if a moved root names a block that is neither in
// batch, kept, nor in the inner store.
func (s *PendingStore) checkRootHoles(ctx context.Context, batch []*PutBatchEntry, roots []NamedRoot) {
	in := make(map[string]struct{}, len(batch))
	for _, e := range batch {
		in[pendingKey(e.Ref)] = struct{}{}
	}
	var refs []*BlockRef
	var names []string
	s.mtx.RLock()
	for _, r := range roots {
		if r.Ref.GetEmpty() {
			continue
		}
		key := pendingKey(r.Ref)
		if _, ok := in[key]; ok || s.kept[key] != nil {
			continue
		}
		refs = append(refs, r.Ref)
		names = append(names, r.Name)
	}
	s.mtx.RUnlock()
	if len(refs) == 0 {
		return
	}
	found, err := s.StoreOps.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return
	}
	for i, ok := range found {
		if ok {
			continue
		}
		sweptMtx.Lock()
		swept := sweptAt[pendingKey(refs[i])]
		sweptMtx.Unlock()
		panic(fmt.Sprintf("HOLE: SetRoots moves root %s onto %s, which is neither kept nor stored\nswept by:\n%s", names[i], refs[i].MarshalString(), swept))
	}
}
