import { setFatalHandler } from '@goscript/builtin/index.js'

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
