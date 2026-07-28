import {
  isBackendEntrypointLifecycle,
  type BackendAPI,
  type BackendEntrypointFunc,
} from '@aptre/bldr-sdk'

// startWorkerPluginEntrypoint waits for startup and observes process-lifetime failure.
export async function startWorkerPluginEntrypoint(
  entrypoint: BackendEntrypointFunc,
  api: BackendAPI,
  abortSignal: AbortSignal,
  runtimeWasmEnv: Record<string, string> | undefined,
  reportRuntimeFailure: (err: unknown) => void,
): Promise<void> {
  const result = entrypoint(api, abortSignal, runtimeWasmEnv)
  if (!isBackendEntrypointLifecycle(result)) {
    await result
    return
  }

  void Promise.resolve(result.done).catch(reportRuntimeFailure)
  await result.startup
}
