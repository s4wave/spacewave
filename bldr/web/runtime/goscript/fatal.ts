import { go, panic, setFatalHandler } from '@goscript/builtin/index.js'

// onGoScriptFatal hands the goroutine failure that ends the GoScript program
// to fail, in place of GoScript's default of raising an uncaught error. It
// prints Go's panic report, with the goroutine and its stack, to the console,
// and passes fail an error whose message is the report's first line, such as
// "panic: <text>". The GoScript runtime starts no goroutine after a failure.
export function onGoScriptFatal(fail: (err: Error) => void): void {
  setFatalHandler((fatal) => {
    if (fatal.message) {
      console.error(fatal.message)
    }
    const summary =
      fatal.message.split('\n', 1)[0] || `exit status ${fatal.exitCode}`
    fail(new Error(summary, { cause: fatal.error }))
  })
}

// RUNTIME_FAULT_ENV names the runtime environment variable that injects a
// fault for end-to-end tests of the failed state. "goroutine-panic" starts a
// goroutine that panics once the program has started, which ends the program
// as a real escaped panic would.
export const RUNTIME_FAULT_ENV = 'BLDR_RUNTIME_FAULT'

// injectRuntimeFault applies the fault env names, if any. Call it just
// before main starts; the fault fires after main's first synchronous run.
export function injectRuntimeFault(
  env: Record<string, string> | undefined,
): void {
  if (env?.[RUNTIME_FAULT_ENV] !== 'goroutine-panic') {
    return
  }
  setTimeout(() => {
    go(() => panic('injected runtime fault'))
  }, 0)
}
