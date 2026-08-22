// DebugContext is the cross-bundle state shared between the app and eval scripts.
// The app calls setDebugContext() at startup; eval scripts call getDebugContext().
export interface DebugContext {
  [key: string]: unknown
}

const GLOBAL_KEY = '__s4wave_debug'

// debugContextStore is the process-global store holding the debug context.
const debugContextStore = globalThis as Record<string, unknown>

// setDebugContext stores the debug context on globalThis for eval scripts to access.
export function setDebugContext(ctx: DebugContext): void {
  debugContextStore[GLOBAL_KEY] = ctx
}

// clearDebugContext removes the debug context when it still matches the
// caller's generation.
export function clearDebugContext(ctx?: DebugContext): void {
  if (ctx && debugContextStore[GLOBAL_KEY] !== ctx) {
    return
  }
  Reflect.deleteProperty(debugContextStore, GLOBAL_KEY)
}

// getDebugContext retrieves the debug context set by the app.
// Throws if the context has not been initialized.
export function getDebugContext<T = DebugContext>(): T {
  const ctx = debugContextStore[GLOBAL_KEY]
  if (!ctx) {
    throw new Error('Debug context not initialized. Is the app running?')
  }
  return ctx as T
}
