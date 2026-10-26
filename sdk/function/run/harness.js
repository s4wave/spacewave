// The harness of one PureFunction run, evaluated as a module in a fresh
// QuickJS-NG instance. The host replaces the input placeholder below with
// the input in protobuf JSON form and reads the last line the harness prints.
//
// The harness fixes the clock and randomness before the function loads, so
// the same input gives the same output on every host, and takes print and the
// console away from the function so only the harness writes to stdout.

const print = globalThis.print

// The function sees the Unix epoch as the current time.
const NativeDate = Date
class FixedDate extends NativeDate {
  constructor(...args) {
    if (args.length === 0) {
      super(0)
    } else {
      super(...args)
    }
  }

  static now() {
    return 0
  }
}
globalThis.Date = new Proxy(FixedDate, {
  // Date() called without new returns the current time as a string.
  apply() {
    return new NativeDate(0).toString()
  },
})

// Math.random is a mulberry32 generator with a fixed seed.
let seed = 0x9e3779b9
Math.random = () => {
  seed = (seed + 0x6d2b79f5) | 0
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296
}

// The performance clock stays at zero.
globalThis.performance = Object.freeze({ timeOrigin: 0, now: () => 0 })

// The function cannot write to stdout or read the host's arguments.
const noop = () => {}
globalThis.console = Object.freeze({
  log: noop,
  info: noop,
  warn: noop,
  error: noop,
  debug: noop,
  trace: noop,
})
for (const name of ['print', 'scriptArgs', 'argv0', 'execArgv']) {
  delete globalThis[name]
}

// Load the function, run it on the input and report the result.
let line
try {
  const { default: fn } = await import('/function.js')
  if (typeof fn !== 'function') {
    throw new TypeError('the module has no default export function')
  }
  const output = await fn(__PURE_FUNCTION_INPUT__)
  line = JSON.stringify({ output: output ?? {} })
} catch (err) {
  const outOfMemory =
    err instanceof InternalError && err.message === 'out of memory'
  let error
  try {
    error = String(err)
  } catch {
    error = 'the function threw a value with no string form'
  }
  line = JSON.stringify({ error, outOfMemory })
}
print(line)
