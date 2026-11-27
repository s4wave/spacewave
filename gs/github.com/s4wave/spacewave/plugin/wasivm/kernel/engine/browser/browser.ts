// Package browser runs kernel guests on the browser's WebAssembly API. See
// the package's Go declaration for the design. This file is its implementation.
//
// A guest call is a Frame. A host import that returns a promise, or that took
// a snapshot, unwinds the guest with Asyncify: the import starts the unwind,
// the export returns, and Function.Call copies the unwound stack out of guest
// memory. Function.Call then awaits the host call, copies the stack back and
// calls the export again to rewind. The rewound import returns the host call's
// results. A snapshot keeps the copied stack, so restoring it rewinds to the
// same import again with other results, in the same instance or in a fork of
// it.

import * as $ from '@goscript/builtin/index.js'
import * as context from '@goscript/context/index.js'

import type * as engine from '@goscript/github.com/s4wave/spacewave/plugin/wasivm/kernel/engine/index.js'

// pageSize is the size of a WebAssembly memory page.
const pageSize = 65536

// unwindPages is the size in pages of the region holding an unwound stack.
const unwindPages = 4

// frameKey is the context key of the Frame a host call runs in.
const frameKey = Symbol('browser.frame')

// Runtime compiles guests with WebAssembly.compile.
export class Runtime {
  // imports maps each defined module name to its host functions.
  private readonly imports = new Map<string, Map<string, engine.HostFunction>>()

  // Define provides fns to guests as the imports of module.
  Define(
    _ctx: context.Context | null,
    module: string,
    fns: Map<string, engine.HostFunction> | null,
  ): $.GoError {
    this.imports.set(module, fns ?? new Map())
    return null
  }

  // Compile compiles a WebAssembly binary into a reusable module.
  async Compile(
    _ctx: context.Context | null,
    wasm: $.Slice<number>,
  ): Promise<[Module | null, $.GoError]> {
    try {
      // GoScript byte slices view an ArrayBuffer, never shared memory.
      const bytes = (
        wasm instanceof Uint8Array ? wasm : new Uint8Array($.asArray(wasm))
      ) as Uint8Array<ArrayBuffer>
      const compiled = await WebAssembly.compile(bytes)
      return [new Module(this.imports, compiled, readExportTypes(bytes)), null]
    } catch (err) {
      return [null, callError(err)]
    }
  }

  // Snapshotter returns the snapshotter of the host call running under ctx,
  // or null when its guest is not transformed with Asyncify.
  Snapshotter(ctx: context.Context | null): Snapshotter | null {
    const frame = ctx?.Value(frameKey) as Frame | null | undefined
    if (!frame?.inst.unwinder) {
      return null
    }
    return new Snapshotter(frame)
  }

  // Close releases the runtime.
  Close(_ctx: context.Context | null): $.GoError {
    return null
  }
}

// New creates a Runtime.
export function New(): Runtime {
  return new Runtime()
}

// Module is a compiled WebAssembly.Module.
export class Module {
  constructor(
    private readonly imports: Map<string, Map<string, engine.HostFunction>>,
    private readonly compiled: WebAssembly.Module,
    private readonly exportTypes: Map<string, FunctionType>,
  ) {}

  // Instantiate creates an instance of the module. The kernel calls the
  // guest's entry itself, so instantiation starts nothing.
  async Instantiate(
    _ctx: context.Context | null,
    _name: string,
  ): Promise<[Instance | null, $.GoError]> {
    try {
      const inst = new Instance(this.exportTypes)
      const imports: WebAssembly.Imports = {}
      for (const [module, fns] of this.imports) {
        const moduleImports: Record<string, WebAssembly.ImportValue> = {}
        for (const [name, fn] of fns) {
          moduleImports[name] = hostImport(inst, `${module}.${name}`, fn)
        }
        imports[module] = moduleImports
      }
      inst.attach(await WebAssembly.instantiate(this.compiled, imports))
      return [inst, null]
    } catch (err) {
      return [null, callError(err)]
    }
  }

  // Close releases the compiled module.
  Close(_ctx: context.Context | null): $.GoError {
    return null
  }
}

// Instance is a WebAssembly.Instance and its active guest calls.
export class Instance {
  // frames are the guest calls in progress, innermost last. A host call may
  // call into the guest again, as a signal handler does.
  readonly frames: Frame[] = []
  // unwinder suspends the guest, or is null when the guest is not
  // transformed with Asyncify.
  unwinder: Unwinder | null = null

  private exports: WebAssembly.Exports = {}
  private memory: Memory | null = null

  constructor(private readonly exportTypes: Map<string, FunctionType>) {}

  // attach binds the instantiated module and reserves the unwind region.
  attach(instance: WebAssembly.Instance): void {
    this.exports = instance.exports
    const memory = this.exports.memory
    if (memory instanceof WebAssembly.Memory) {
      this.memory = new Memory(memory)
      this.unwinder = Unwinder.attach(this.exports, memory)
    }
  }

  // frame returns the innermost guest call.
  frame(): Frame {
    const frame = this.frames.at(-1)
    if (!frame) {
      throw $.newError('host call outside a guest call')
    }
    return frame
  }

  // export returns the export name and its type, or null.
  export(name: string): { fn: WasmFunction; type: FunctionType } | null {
    const fn = this.exports[name]
    const type = this.exportTypes.get(name)
    if (typeof fn !== 'function' || !type) {
      return null
    }
    return { fn: fn as WasmFunction, type }
  }

  // Memory returns the instance's linear memory.
  Memory(): Memory | null {
    return this.memory
  }

  // ExportedFunction returns an exported function, or null.
  ExportedFunction(name: string): Function | null {
    return this.export(name) ? new Function(this, name) : null
  }

  // ExportedGlobal returns an exported global, or null.
  ExportedGlobal(name: string): Global | null {
    const global = this.exports[name]
    return global instanceof WebAssembly.Global ? new Global(global) : null
  }

  // Close stops the instance.
  Close(_ctx: context.Context | null): $.GoError {
    return null
  }
}

// Function is an exported guest function. Its calls suspend across
// asynchronous host calls.
export class Function {
  constructor(
    private readonly inst: Instance,
    private readonly name: string,
  ) {}

  // Call invokes the function.
  Call(
    ctx: context.Context | null,
    args: $.Slice<bigint>,
  ): Promise<[$.Slice<bigint>, $.GoError]> {
    return callExport(this.inst, ctx, this.name, [...$.asArray(args)], null, [])
  }
}

// callExport runs the export name of inst with args as a new frame of inst. It
// unwinds and rewinds the guest at each host call that awaits or takes a
// snapshot, and at each snapshot restore that targets this call. When rewind
// is set, the call first rewinds to that unwound stack, whose host import
// then returns resume.
async function callExport(
  inst: Instance,
  ctx: context.Context | null,
  name: string,
  args: bigint[],
  rewind: Uint8Array | null,
  resume: bigint[],
): Promise<[$.Slice<bigint>, $.GoError]> {
  // Push the frame.
  const exp = inst.export(name)
  if (!exp) {
    return [null, $.newError(`call of missing export ${name}`)]
  }
  const frame = new Frame(inst, ctx, name, args)
  const wasmArgs = args.map((arg, i) => toWasm(arg, exp.type.params[i]))
  inst.frames.push(frame)
  if (rewind) {
    frame.resume = resume
    inst.unwinder!.startRewind(rewind)
  }

  try {
    for (;;) {
      try {
        // Run the guest until it returns or unwinds.
        const result = exp.fn(...wasmArgs)
        const capture = frame.unwound
        if (!capture) {
          return [fromWasmResults(result, exp.type.results), null]
        }
        frame.unwound = null

        // Save the unwound stack, finish the host call, and rewind.
        const unwinder = inst.unwinder!
        capture.complete(unwinder.stopUnwind())
        frame.resume = await frame.pending
        unwinder.startRewind(capture.stack!)
      } catch (err) {
        // A restore targeting this call rewinds to its snapshot.
        frame.abandon()
        if (!(err instanceof Restore) || err.frame !== frame) {
          if (err instanceof Restore && inst.frames.includes(err.frame)) {
            throw err
          }
          return [null, callError(err)]
        }
        frame.resume = err.results
        inst.unwinder!.startRewind(err.stack)
      }
    }
  } finally {
    inst.frames.pop()
  }
}

// Frame is one guest call in progress.
class Frame {
  // ctx is the call's context, through which its host calls find it.
  readonly ctx: context.Context
  // depth is the frame's index in the instance's frames.
  readonly depth: number
  // capture is the state of the host call running in this frame.
  capture: Capture | null = null
  // unwound is the capture of a host call the guest is unwinding from.
  unwound: Capture | null = null
  // pending settles to the stack of the host call the guest unwound from.
  pending: bigint[] | Promise<bigint[]> = []
  // resume is the stack the rewound host import returns, or null when the
  // guest is not rewinding.
  resume: bigint[] | null = null

  constructor(
    readonly inst: Instance,
    ctx: context.Context | null,
    // name and args are the export the call runs and its arguments.
    // A fork resumes the call with name and args.
    readonly name: string,
    readonly args: bigint[],
  ) {
    this.ctx = context.WithValue(ctx, frameKey, this)
    this.depth = inst.frames.length
  }

  // abandon ends the captures of a guest call that failed, so a fork waiting
  // to resume from one fails too.
  abandon(): void {
    this.capture?.complete(null)
    this.unwound?.complete(null)
    this.capture = null
    this.unwound = null
  }
}

// Capture is the guest stack unwound at one host call.
class Capture {
  // wanted reports that a snapshot needs the stack, so the guest unwinds
  // even when the host call finishes at once.
  wanted = false
  // stack is the unwound stack, or null before the guest unwinds.
  stack: Uint8Array | null = null
  // done settles once stack is set, or once the guest call failed without
  // completing the capture.
  readonly done: Promise<void>
  private settle: () => void = () => {}

  constructor(
    // depth, name and args are those of the frame the host call ran in.
    readonly depth: number,
    readonly name: string,
    readonly args: bigint[],
  ) {
    this.done = new Promise((resolve) => {
      this.settle = resolve
    })
  }

  // complete records the unwound stack, or null when the guest call failed
  // first.
  complete(stack: Uint8Array | null): void {
    this.stack ??= stack
    this.settle()
  }
}

// hostImport adapts a kernel host function to a WebAssembly import of inst.
function hostImport(
  inst: Instance,
  name: string,
  fn: engine.HostFunction,
): (...args: WasmValue[]) => WasmValue | WasmValue[] | undefined {
  const params = $.asArray(fn.Params)
  const results = $.asArray(fn.Results)
  const call = fn.Call
  return (...args) => {
    // A rewinding guest re-enters the import it unwound from, which returns
    // the results of the finished host call.
    const frame = inst.frame()
    if (frame.resume) {
      inst.unwinder!.stopRewind()
      const stack = frame.resume
      frame.resume = null
      return toWasmResults(stack, results)
    }

    // Stop a guest whose call context ended. A blocked host call returns
    // once the context ends, so the guest stops at its next host call.
    const ctxErr = frame.ctx!.Err()
    if (ctxErr) {
      throw ctxErr
    }

    // Run the host call.
    if (!call) {
      throw $.newError(`${name} has no host function`)
    }
    const stack = Array.from(
      { length: Math.max(params.length, results.length) },
      () => 0n,
    )
    args.forEach((arg, i) => {
      stack[i] = fromWasm(arg, params[i])
    })
    const capture = new Capture(frame.depth, frame.name, frame.args)
    frame.capture = capture
    const err = call(frame.ctx, inst, stack)

    // Unwind to await the call or to capture the stack for a snapshot.
    if (isPromise(err)) {
      frame.pending = err.then((err) => {
        if (err) {
          throw err
        }
        return stack
      })
      return unwind(inst, frame, capture, name, results)
    }
    if (err) {
      throw err
    }
    if (capture.wanted) {
      frame.pending = stack
      return unwind(inst, frame, capture, name, results)
    }
    return toWasmResults(stack, results)
  }
}

// unwind starts unwinding the guest from a host import and returns the
// placeholder results the unwinding guest ignores.
function unwind(
  inst: Instance,
  frame: Frame,
  capture: Capture,
  name: string,
  results: number[],
): WasmValue | WasmValue[] | undefined {
  if (!inst.unwinder) {
    void Promise.resolve(frame.pending).catch(() => {})
    throw $.newError(
      `${name} must suspend the program, which needs wasm-opt --asyncify`,
    )
  }
  frame.unwound = capture
  inst.unwinder.startUnwind()
  return toWasmResults([], results)
}

// Snapshotter captures the state of the host call running in a frame.
class Snapshotter {
  constructor(private readonly frame: Frame) {}

  // Snapshot captures the call stack of the running host call.
  Snapshot(): Snapshot {
    const capture = this.frame.capture!
    capture.wanted = true
    return new Snapshot(capture)
  }
}

// Snapshot is the guest stack of one host call.
class Snapshot {
  constructor(private readonly capture: Capture) {}

  // Restore resumes the guest call running under ctx at the capture,
  // returning returnValues from the host function that took the capture.
  // Restore does not return: the thrown Restore unwinds to the call of the
  // frame at the capture's depth.
  Restore(ctx: context.Context | null, returnValues: $.Slice<bigint>): void {
    const frame = ctx?.Value(frameKey) as Frame | null | undefined
    const target = frame?.inst.frames[this.capture.depth]
    if (!target) {
      throw $.newError('restore outside the guest call of its snapshot')
    }
    const stack = this.capture.stack
    if (!stack) {
      throw $.newError('restore of a snapshot the guest has not unwound to')
    }
    throw new Restore(target, stack, [...$.asArray(returnValues)])
  }

  // Resume runs the captured guest call in inst, a fork of the instance the
  // capture was taken in, as if the host function that took the capture
  // returned returnValues. It waits for the capture and returns when the call
  // does.
  async Resume(
    ctx: context.Context | null,
    inst: engine.Instance | null,
    returnValues: $.Slice<bigint>,
  ): Promise<[$.Slice<bigint>, $.GoError]> {
    // Wait for the capture.
    const capture = this.capture
    await capture.done
    if (!capture.stack) {
      return [
        null,
        $.newError('resume of a snapshot the guest has not unwound to'),
      ]
    }

    // Rewind a new outermost frame of inst to the captured stack.
    if (
      !(inst instanceof Instance) ||
      !inst.unwinder ||
      capture.depth !== inst.frames.length
    ) {
      return [
        null,
        $.newError("resume outside a fork of the snapshot's instance"),
      ]
    }
    return callExport(inst, ctx, capture.name, capture.args, capture.stack, [
      ...$.asArray(returnValues),
    ])
  }
}

// Restore carries a snapshot restore to the Function.Call of its frame.
class Restore {
  constructor(
    readonly frame: Frame,
    readonly stack: Uint8Array,
    readonly results: bigint[],
  ) {}
}

// Unwinder drives a guest's Asyncify exports. The unwound stack lives in a
// region of guest memory reserved past the guest's own pages. Between unwind
// and rewind, the stack is held as a copy, so a nested call can reuse the
// region.
class Unwinder {
  private constructor(
    private readonly asyncify: AsyncifyExports,
    private readonly memory: WebAssembly.Memory,
    private readonly base: number,
  ) {}

  // attach reserves the unwind region of an Asyncify guest, or returns null
  // when the guest is not transformed with Asyncify.
  static attach(
    exports: WebAssembly.Exports,
    memory: WebAssembly.Memory,
  ): Unwinder | null {
    if (typeof exports.asyncify_start_unwind !== 'function') {
      return null
    }
    const base = memory.grow(unwindPages) * pageSize
    const unwinder = new Unwinder(
      exports as unknown as AsyncifyExports,
      memory,
      base,
    )
    unwinder.setHeader(0)
    return unwinder
  }

  // startUnwind starts unwinding the guest from a host import.
  startUnwind(): void {
    this.asyncify.asyncify_start_unwind(this.base)
  }

  // stopUnwind ends the unwind once the export returned and returns a copy
  // of the unwound stack.
  stopUnwind(): Uint8Array {
    this.asyncify.asyncify_stop_unwind()
    const view = new DataView(this.memory.buffer)
    const end = view.getUint32(this.base, true)
    const stack = new Uint8Array(
      this.memory.buffer,
      this.base + 8,
      end - this.base - 8,
    ).slice()
    this.setHeader(0)
    return stack
  }

  // startRewind loads an unwound stack and starts rewinding to its import.
  // The caller then calls the same export again.
  startRewind(stack: Uint8Array): void {
    new Uint8Array(this.memory.buffer, this.base + 8, stack.length).set(stack)
    this.setHeader(stack.length)
    this.asyncify.asyncify_start_rewind(this.base)
  }

  // stopRewind ends the rewind at the import the guest unwound from.
  stopRewind(): void {
    this.asyncify.asyncify_stop_rewind()
  }

  // setHeader writes the Asyncify data header for a stack of used bytes: the
  // stack's current end and the region's end.
  private setHeader(used: number): void {
    const view = new DataView(this.memory.buffer)
    view.setUint32(this.base, this.base + 8 + used, true)
    view.setUint32(this.base + 4, this.base + unwindPages * pageSize, true)
  }
}

// AsyncifyExports are the exports Asyncify adds to a guest.
type AsyncifyExports = {
  asyncify_start_unwind(data: number): void
  asyncify_stop_unwind(): void
  asyncify_start_rewind(data: number): void
  asyncify_stop_rewind(): void
}

// Global is an exported guest global.
class Global {
  constructor(private readonly global: WebAssembly.Global) {}

  // Get returns the current value.
  Get(): bigint {
    const value = this.global.value as number | bigint
    return typeof value === 'bigint'
      ? BigInt.asUintN(64, value)
      : BigInt(value >>> 0)
  }

  // Set changes the value.
  Set(val: bigint): void {
    this.global.value =
      typeof this.global.value === 'bigint'
        ? BigInt.asIntN(64, val)
        : Number(BigInt.asIntN(32, val))
  }
}

// Memory is a guest's linear memory. Each access reports false when it falls
// outside the memory.
class Memory {
  constructor(private readonly mem: WebAssembly.Memory) {}

  // Read returns length bytes at offset, aliasing guest memory.
  Read(offset: number, length: number): [$.Slice<number>, boolean] {
    if (offset + length > this.mem.buffer.byteLength) {
      return [null, false]
    }
    return [new Uint8Array(this.mem.buffer, offset, length), true]
  }

  // ReadByteAt returns the byte at offset.
  ReadByteAt(offset: number): [number, boolean] {
    if (offset >= this.mem.buffer.byteLength) {
      return [0, false]
    }
    return [new Uint8Array(this.mem.buffer)[offset], true]
  }

  // ReadUint32Le returns the little-endian uint32 at offset.
  ReadUint32Le(offset: number): [number, boolean] {
    if (offset + 4 > this.mem.buffer.byteLength) {
      return [0, false]
    }
    return [new DataView(this.mem.buffer).getUint32(offset, true), true]
  }

  // ReadUint64Le returns the little-endian uint64 at offset.
  ReadUint64Le(offset: number): [bigint, boolean] {
    if (offset + 8 > this.mem.buffer.byteLength) {
      return [0n, false]
    }
    return [new DataView(this.mem.buffer).getBigUint64(offset, true), true]
  }

  // Write copies data to offset.
  Write(offset: number, data: $.Slice<number>): boolean {
    const bytes =
      data instanceof Uint8Array ? data : new Uint8Array($.asArray(data))
    if (offset + bytes.length > this.mem.buffer.byteLength) {
      return false
    }
    new Uint8Array(this.mem.buffer, offset, bytes.length).set(bytes)
    return true
  }

  // WriteByteAt writes one byte at offset.
  WriteByteAt(offset: number, val: number): boolean {
    if (offset >= this.mem.buffer.byteLength) {
      return false
    }
    new Uint8Array(this.mem.buffer)[offset] = val
    return true
  }

  // WriteUint32Le writes a little-endian uint32 at offset.
  WriteUint32Le(offset: number, val: number): boolean {
    if (offset + 4 > this.mem.buffer.byteLength) {
      return false
    }
    new DataView(this.mem.buffer).setUint32(offset, val, true)
    return true
  }

  // WriteUint64Le writes a little-endian uint64 at offset.
  WriteUint64Le(offset: number, val: bigint): boolean {
    if (offset + 8 > this.mem.buffer.byteLength) {
      return false
    }
    new DataView(this.mem.buffer).setBigUint64(offset, val, true)
    return true
  }

  // Size returns the memory size in bytes.
  Size(): number {
    return this.mem.buffer.byteLength
  }

  // Grow adds pages and returns the previous size in pages.
  Grow(pages: number): [number, boolean] {
    try {
      return [this.mem.grow(pages), true]
    } catch {
      return [0, false]
    }
  }
}

// WasmValue is a WebAssembly value as JavaScript passes it.
type WasmValue = number | bigint

// WasmFunction is an exported WebAssembly function.
type WasmFunction = (...args: WasmValue[]) => unknown

// FunctionType is a function's parameter and result types, in their binary
// encoding.
type FunctionType = {
  params: number[]
  results: number[]
}

// i32, i64, f32 and f64 are value types in their binary encoding.
const i32 = 0x7f
const i64 = 0x7e
const f32 = 0x7d
const f64 = 0x7c

// fromWasm converts a WebAssembly value to its uint64 stack encoding.
function fromWasm(value: WasmValue, type: number): bigint {
  switch (type) {
    case i32:
      return BigInt((value as number) >>> 0)
    case i64:
      return BigInt.asUintN(64, value as bigint)
    case f32: {
      const view = new DataView(new ArrayBuffer(4))
      view.setFloat32(0, value as number, true)
      return BigInt(view.getUint32(0, true))
    }
    case f64: {
      const view = new DataView(new ArrayBuffer(8))
      view.setFloat64(0, value as number, true)
      return view.getBigUint64(0, true)
    }
    default:
      throw $.newError(
        `unsupported WebAssembly value type 0x${type.toString(16)}`,
      )
  }
}

// toWasm converts a uint64 stack value to a WebAssembly value of type.
function toWasm(value: bigint, type: number): WasmValue {
  switch (type) {
    case i32:
      return Number(BigInt.asIntN(32, value))
    case i64:
      return BigInt.asIntN(64, value)
    case f32: {
      const view = new DataView(new ArrayBuffer(4))
      view.setUint32(0, Number(BigInt.asUintN(32, value)), true)
      return view.getFloat32(0, true)
    }
    case f64: {
      const view = new DataView(new ArrayBuffer(8))
      view.setBigUint64(0, BigInt.asUintN(64, value), true)
      return view.getFloat64(0, true)
    }
    default:
      throw $.newError(
        `unsupported WebAssembly value type 0x${type.toString(16)}`,
      )
  }
}

// toWasmResults converts the leading stack values to an import's results.
function toWasmResults(
  stack: bigint[],
  types: number[],
): WasmValue | WasmValue[] | undefined {
  const values = types.map((type, i) => toWasm(stack[i] ?? 0n, type))
  return values.length > 1 ? values : values[0]
}

// fromWasmResults converts an export's results to stack values.
function fromWasmResults(result: unknown, types: number[]): $.Slice<bigint> {
  if (types.length === 0) {
    return null
  }
  const values = types.length === 1 ? [result] : (result as WasmValue[])
  return values.map((value, i) => fromWasm(value as WasmValue, types[i]))
}

// callError converts what a guest call threw to its Go error. A Go panic in a
// host call, such as the one that ends a process, yields the panic's error.
function callError(err: unknown): $.GoError {
  if (err instanceof $.GoPanic) {
    $.recovered(err)
    err = err.value
  }
  if (isGoError(err)) {
    return err
  }
  return err instanceof Error ? $.toGoError(err) : $.newError(String(err))
}

// isGoError reports whether value implements the Go error interface.
function isGoError(value: unknown): value is Exclude<$.GoError, null> {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { Error?: unknown }).Error === 'function'
  )
}

// isPromise reports whether value is a promise.
function isPromise(value: unknown): value is Promise<$.GoError> {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { then?: unknown }).then === 'function'
  )
}

// readExportTypes reads the type of each exported function of a module.
function readExportTypes(bytes: Uint8Array): Map<string, FunctionType> {
  const types: FunctionType[] = []
  const funcTypes: number[] = []
  const exports = new Map<string, number>()
  const reader = new WasmReader(bytes, 8)
  while (!reader.done()) {
    const id = reader.byte()
    const section = reader.section()
    switch (id) {
      case 1:
        section.vector(() => {
          section.byte()
          types.push({
            params: section.vector(() => section.byte()),
            results: section.vector(() => section.byte()),
          })
        })
        break
      case 2:
        section.vector(() => {
          section.name()
          section.name()
          const kind = section.byte()
          if (kind === 0) {
            funcTypes.push(section.uint())
          } else {
            section.skipImport(kind)
          }
        })
        break
      case 3:
        funcTypes.push(...section.vector(() => section.uint()))
        break
      case 7:
        section.vector(() => {
          const name = section.name()
          const kind = section.byte()
          const index = section.uint()
          if (kind === 0) {
            exports.set(name, index)
          }
        })
        break
    }
  }

  const exportTypes = new Map<string, FunctionType>()
  for (const [name, index] of exports) {
    exportTypes.set(name, types[funcTypes[index]])
  }
  return exportTypes
}

// WasmReader reads the binary encoding of a WebAssembly module.
class WasmReader {
  constructor(
    private readonly bytes: Uint8Array,
    private offset = 0,
  ) {}

  // done reports whether every byte has been read.
  done(): boolean {
    return this.offset >= this.bytes.length
  }

  // byte reads one byte.
  byte(): number {
    if (this.offset >= this.bytes.length) {
      throw $.newError('unexpected end of the WebAssembly module')
    }
    return this.bytes[this.offset++]
  }

  // uint reads an unsigned LEB128 integer.
  uint(): number {
    let value = 0
    for (let shift = 0; ; shift += 7) {
      const b = this.byte()
      value += (b & 0x7f) * 2 ** shift
      if ((b & 0x80) === 0) {
        return value
      }
    }
  }

  // name reads a length-prefixed UTF-8 name.
  name(): string {
    const length = this.uint()
    const start = this.offset
    this.offset += length
    return new TextDecoder().decode(this.bytes.subarray(start, this.offset))
  }

  // vector reads a count and that many items.
  vector<T>(read: () => T): T[] {
    return Array.from({ length: this.uint() }, read)
  }

  // section reads a section's size and returns a reader over its contents.
  section(): WasmReader {
    const length = this.uint()
    const start = this.offset
    this.offset += length
    return new WasmReader(this.bytes.subarray(start, this.offset))
  }

  // skipImport skips the description of a table, memory or global import.
  skipImport(kind: number): void {
    switch (kind) {
      case 1:
        this.byte()
        this.skipLimits()
        return
      case 2:
        this.skipLimits()
        return
      case 3:
        this.byte()
        this.byte()
        return
      default:
        throw $.newError(`unsupported WebAssembly import kind ${kind}`)
    }
  }

  // skipLimits skips a table or memory's limits.
  private skipLimits(): void {
    const flags = this.uint()
    this.uint()
    if ((flags & 1) !== 0) {
      this.uint()
    }
  }
}
