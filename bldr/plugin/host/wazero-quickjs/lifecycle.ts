import {
  isBackendEntrypointLifecycle,
  type BackendAPI,
  type BackendEntrypointFunc,
} from '../../../sdk/plugin.js'

// startBackendEntrypoint applies the shared backend startup contract and
// returns the completion promise that keeps the QuickJS event loop alive.
export async function startBackendEntrypoint(
  main: BackendEntrypointFunc,
  api: BackendAPI,
  signal: AbortSignal,
): Promise<{ done: Promise<void> } | undefined> {
  const result = main(api, signal)
  if (!isBackendEntrypointLifecycle(result)) {
    await result
    return undefined
  }

  await result.startup
  return result.done ? { done: result.done } : undefined
}
