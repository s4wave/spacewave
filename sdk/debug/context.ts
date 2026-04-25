// DebugContext is the cross-bundle state shared between the app and eval scripts.
// The app calls setDebugContext() at startup; eval scripts call getDebugContext().
export interface DebugContext {
  [key: string]: unknown
}

// TestDebugContext is the typed debug context for e2e test scripts.
// Fields match the setDebugContext() call in app/AppAPI.tsx.
export interface TestDebugContext extends DebugContext {
  client: import('@aptre/bldr-sdk/resource/index.js').Client
  root: import('../root/root.js').Root
  createLocalSession: typeof import('../../app/quickstart/create.js').createLocalSession
  createDrive: typeof import('../../app/quickstart/create.js').createDrive
  createQuickstartSetup: typeof import('../../app/quickstart/create.js').createQuickstartSetup
  mountSpace: typeof import('../../app/space/space.js').mountSpace
  FSHandle: typeof import('../unixfs/handle.js').FSHandle
  SpacewaveProvider: typeof import('../provider/spacewave/spacewave.js').SpacewaveProvider
  UNIXFS_OBJECT_KEY: string
  runSOPerfTest: typeof import('../../app/quickstart/perf-test.js').runSOPerfTest
}

const GLOBAL_KEY = '__s4wave_debug'

// globalStore returns globalThis typed for debug context storage.
function globalStore(): Record<string, unknown> {
  return globalThis as never
}

// setDebugContext stores the debug context on globalThis for eval scripts to access.
export function setDebugContext(ctx: DebugContext): void {
  globalStore()[GLOBAL_KEY] = ctx
}

// getDebugContext retrieves the debug context set by the app.
// Throws if the context has not been initialized.
export function getDebugContext<T = DebugContext>(): T {
  const ctx = globalStore()[GLOBAL_KEY]
  if (!ctx) {
    throw new Error('Debug context not initialized. Is the app running?')
  }
  return ctx as T
}
