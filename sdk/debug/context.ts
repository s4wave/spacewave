// DebugContext is the cross-bundle state shared between the app and eval scripts.
// The app calls setDebugContext() at startup.
// Eval scripts call getDebugContext().
export interface DebugContext {
  [key: string]: unknown
}

const GLOBAL_KEY = '__s4wave_debug'
const APPS_KEY = '__s4wave_debug_apps'

// debugContextStore is the process-global store holding the debug context.
const debugContextStore = globalThis as Record<string, unknown>

// setDebugContext stores the debug context on globalThis for eval scripts to access.
export function setDebugContext(ctx: DebugContext, appId = ''): void {
  if (appId) {
    appContexts().set(appId, ctx)
    return
  }
  debugContextStore[GLOBAL_KEY] = ctx
}

// clearDebugContext removes the debug context when it still matches the
// caller's generation.
export function clearDebugContext(ctx?: DebugContext, appId = ''): void {
  if (appId) {
    const apps = appContexts()
    if (!ctx || apps.get(appId) === ctx) apps.delete(appId)
    return
  }
  if (ctx && debugContextStore[GLOBAL_KEY] !== ctx) {
    return
  }
  Reflect.deleteProperty(debugContextStore, GLOBAL_KEY)
}

// getDebugContext retrieves the debug context set by the app.
// getDebugContext throws if the context has not been initialized.
export function getDebugContext<T = DebugContext>(appId = ''): T {
  const ctx = appId ? appContexts().get(appId) : debugContextStore[GLOBAL_KEY]
  if (!ctx) {
    throw new Error('Debug context not initialized. Is the app running?')
  }
  return ctx as T
}

function appContexts(): Map<string, DebugContext> {
  let contexts = debugContextStore[APPS_KEY] as
    | Map<string, DebugContext>
    | undefined
  if (!contexts) {
    contexts = new Map()
    debugContextStore[APPS_KEY] = contexts
  }
  return contexts
}

export function listDebugAppIds(): string[] {
  return [...appContexts().keys()]
}
