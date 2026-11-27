package native

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero/api"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// A guest transformed with Binaryen's Asyncify pass (wasm-opt --asyncify)
// can unwind its call stack into a buffer and rewind it later. Host calls
// block their goroutine, so the engine unwinds only for a snapshot: the host
// import starts the unwind, the export returns, and the guest call copies the
// unwound stack out of guest memory. It then copies the stack back and calls
// the export again to rewind. The rewound import returns the host call's
// results. Restoring the copy rewinds to the same import with other results,
// in the same instance or in a fork of it.

// pageSize is the size of a WebAssembly memory page.
const pageSize = 65536

// unwindPages is the size in pages of the region holding an unwound stack.
const unwindPages = 4

// errNotUnwound is the error of restoring a capture before the guest unwound
// to it, or after its host call failed.
var errNotUnwound = errors.New("restore of a snapshot the guest has not unwound to")

// hostFunc is a kernel host function.
type hostFunc = func(ctx context.Context, inst engine.Instance, stack []uint64) error

// frameKey is the context key of the frame a host call runs in.
type frameKey struct{}

// frame is one guest call in progress.
type frame struct {
	inst *Instance
	// depth is the frame's index in the instance's frames.
	depth int
	// name, fn and args are the export the call runs and its arguments.
	// A rewind calls the export again.
	name string
	fn   api.Function
	args []uint64

	// capture is the capture the running host call took, or nil.
	capture *capture
	// unwound is the capture the guest is unwinding to, or nil.
	unwound *capture
	// rewinding and resume record whether the guest is rewinding to a host
	// import and the results that import returns, respectively.
	rewinding bool
	resume    []uint64
}

// frameOf returns the frame of the host call running under ctx, or nil.
func frameOf(ctx context.Context) *frame {
	fr, _ := ctx.Value(frameKey{}).(*frame)
	return fr
}

// call runs fn, the export name of i, with args as a new frame of i. When
// rewind is set, the call first rewinds to that unwound stack, whose host
// import then returns resume.
func (i *Instance) call(ctx context.Context, name string, fn api.Function, args []uint64, rewind []byte, resume []uint64) ([]uint64, error) {
	// Push the frame, rewinding it first when it resumes an unwound stack.
	fr := &frame{inst: i, depth: len(i.frames), name: name, fn: fn, args: args}
	ctx = context.WithValue(ctx, frameKey{}, fr)
	i.frames = append(i.frames, fr)
	defer func() { i.frames = i.frames[:fr.depth] }()
	if rewind != nil {
		if err := fr.rewind(ctx, rewind, resume); err != nil {
			return nil, err
		}
	}

	// Call the export until the guest returns. A restore that targets this
	// frame, or a capture the guest unwound to, rewinds it and calls again.
	for {
		// Run the guest until it returns, unwinds or fails.
		results, err := fn.Call(ctx, args...)
		if err != nil {
			fr.abandon()
			var rs *restore
			if !errors.As(err, &rs) || rs.frame != fr {
				return nil, err
			}
			if err := fr.rewind(ctx, rs.stack, rs.results); err != nil {
				return nil, err
			}
			continue
		}
		c := fr.unwound
		if c == nil {
			return results, nil
		}

		// Complete the capture and rewind to its host call.
		fr.unwound = nil
		stack, err := i.unwinder.stopUnwind(ctx)
		if err != nil {
			close(c.done)
			return nil, err
		}
		c.stack = stack
		close(c.done)
		if err := fr.rewind(ctx, stack, c.results); err != nil {
			return nil, err
		}
	}
}

// rewind starts rewinding the frame's guest to the host import of stack,
// which then returns resume. The caller then calls the export again.
func (fr *frame) rewind(ctx context.Context, stack []byte, resume []uint64) error {
	fr.rewinding = true
	fr.resume = resume
	return fr.inst.unwinder.startRewind(ctx, stack)
}

// abandon ends the captures of a guest call that failed, so a fork waiting
// to resume from one fails too.
func (fr *frame) abandon() {
	for _, c := range []*capture{fr.capture, fr.unwound} {
		if c != nil {
			close(c.done)
		}
	}
	fr.capture, fr.unwound = nil, nil
}

// hostCall runs the host function fn in the frame, with its arguments and
// results in stack. A rewinding guest re-enters the import it unwound from, which
// returns the finished call's results. A host call that took a snapshot
// unwinds the guest.
func (fr *frame) hostCall(ctx context.Context, fn hostFunc, stack []uint64) error {
	// Return the results of the call the guest rewinds to.
	if fr.rewinding {
		fr.rewinding = false
		copy(stack, fr.resume)
		fr.resume = nil
		return fr.inst.unwinder.stopRewind(ctx)
	}

	// Run the call, then unwind to complete a capture it took.
	if err := fn(ctx, fr.inst, stack); err != nil {
		return err
	}
	c := fr.capture
	if c == nil {
		return nil
	}
	fr.capture = nil
	c.results = slices.Clone(stack)
	fr.unwound = c
	return fr.inst.unwinder.startUnwind(ctx)
}

// capture is the guest stack unwound at one host call.
type capture struct {
	// depth, name and args are those of the frame the host call ran in.
	depth int
	name  string
	args  []uint64
	// results are the host call's results, which the rewind after the
	// unwind returns.
	results []uint64

	// done closes once stack is set, or once the guest call failed without
	// completing the capture.
	done chan struct{}
	// stack is the unwound stack.
	stack []byte
}

// snapshotter captures the state of the host call running in a frame.
type snapshotter struct {
	fr *frame
}

// Snapshot captures the call stack of the running host call.
func (s snapshotter) Snapshot() engine.Snapshot {
	fr := s.fr
	if fr.capture == nil {
		fr.capture = &capture{
			depth: fr.depth,
			name:  fr.name,
			args:  fr.args,
			done:  make(chan struct{}),
		}
	}
	return snapshot{c: fr.capture}
}

// snapshot is the guest stack of one host call.
type snapshot struct {
	c *capture
}

// Restore resumes the guest call running under ctx at the capture. It does
// not return: the restore panics through the guest to the frame's call.
func (s snapshot) Restore(ctx context.Context, returnValues []uint64) {
	// Find the frame at the capture's depth.
	fr := frameOf(ctx)
	if fr == nil || s.c.depth >= len(fr.inst.frames) {
		panic(errors.New("restore outside the guest call of its snapshot"))
	}
	if s.c.stack == nil {
		panic(errNotUnwound)
	}
	panic(&restore{
		frame:   fr.inst.frames[s.c.depth],
		stack:   s.c.stack,
		results: slices.Clone(returnValues),
	})
}

// Resume runs the captured guest call in inst, a fork of the instance the
// capture was taken in.
func (s snapshot) Resume(ctx context.Context, inst engine.Instance, returnValues []uint64) ([]uint64, error) {
	// Wait for the capture.
	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case <-s.c.done:
	}
	if s.c.stack == nil {
		return nil, errNotUnwound
	}

	// Rewind a new outermost frame of inst to it.
	target, ok := inst.(*Instance)
	if !ok || target.unwinder == nil || s.c.depth != len(target.frames) {
		return nil, errors.New("resume outside a fork of the snapshot's instance")
	}
	fn := target.mod.ExportedFunction(s.c.name)
	if fn == nil {
		return nil, errors.Errorf("resume of missing export %s", s.c.name)
	}
	return target.call(ctx, s.c.name, fn, s.c.args, s.c.stack, slices.Clone(returnValues))
}

// restore carries a snapshot restore to the call of its frame.
type restore struct {
	frame   *frame
	stack   []byte
	results []uint64
}

// Error describes the restore, which a guest call returns until it reaches
// its frame.
func (r *restore) Error() string {
	return "snapshot restore in progress"
}

// unwinder drives a guest's Asyncify exports. The unwound stack lives in a
// region of guest memory reserved past the guest's own pages. Between unwind
// and rewind the stack is held as a copy, so a nested call can reuse the
// region.
type unwinder struct {
	startUnwindFn api.Function
	stopUnwindFn  api.Function
	startRewindFn api.Function
	stopRewindFn  api.Function
	mem           api.Memory
	// base is the offset of the region.
	base uint32
}

// attachUnwinder reserves the unwind region of an Asyncify guest. It returns
// nil when the guest is not transformed with Asyncify.
func attachUnwinder(mod api.Module) (*unwinder, error) {
	// Find the Asyncify exports.
	u := &unwinder{
		startUnwindFn: mod.ExportedFunction("asyncify_start_unwind"),
		stopUnwindFn:  mod.ExportedFunction("asyncify_stop_unwind"),
		startRewindFn: mod.ExportedFunction("asyncify_start_rewind"),
		stopRewindFn:  mod.ExportedFunction("asyncify_stop_rewind"),
		mem:           mod.Memory(),
	}
	if u.startUnwindFn == nil || u.stopUnwindFn == nil || u.startRewindFn == nil || u.stopRewindFn == nil || u.mem == nil {
		return nil, nil
	}

	// Reserve the region.
	pages, ok := u.mem.Grow(unwindPages)
	if !ok {
		return nil, errors.New("reserve the unwind region")
	}
	u.base = pages * pageSize
	u.setHeader(0)
	return u, nil
}

// startUnwind starts unwinding the guest from a host import.
func (u *unwinder) startUnwind(ctx context.Context) error {
	_, err := u.startUnwindFn.Call(ctx, uint64(u.base))
	return err
}

// stopUnwind ends the unwind once the export returned and returns a copy of
// the unwound stack.
func (u *unwinder) stopUnwind(ctx context.Context) ([]byte, error) {
	// End the unwind.
	if _, err := u.stopUnwindFn.Call(ctx); err != nil {
		return nil, err
	}

	// Copy the stack out of the region and empty it.
	end, _ := u.mem.ReadUint32Le(u.base)
	stack, ok := u.mem.Read(u.base+8, end-u.base-8)
	if !ok {
		return nil, errors.New("unwound stack outside the unwind region")
	}
	stack = slices.Clone(stack)
	u.setHeader(0)
	return stack, nil
}

// startRewind loads an unwound stack and starts rewinding to its import.
func (u *unwinder) startRewind(ctx context.Context, stack []byte) error {
	u.mem.Write(u.base+8, stack)
	u.setHeader(uint32(len(stack)))
	_, err := u.startRewindFn.Call(ctx, uint64(u.base))
	return err
}

// stopRewind ends the rewind at the import the guest unwound from.
func (u *unwinder) stopRewind(ctx context.Context) error {
	_, err := u.stopRewindFn.Call(ctx)
	return err
}

// setHeader writes the Asyncify data header for a stack of used bytes: the
// stack's current end and the region's end.
func (u *unwinder) setHeader(used uint32) {
	u.mem.WriteUint32Le(u.base, u.base+8+used)
	u.mem.WriteUint32Le(u.base+4, u.base+unwindPages*pageSize)
}

// _ is a type assertion
var (
	_ engine.Snapshotter = snapshotter{}
	_ engine.Snapshot    = snapshot{}
)
