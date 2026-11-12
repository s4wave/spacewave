import { Server, createMux } from 'starpc'
import {
  isBackendEntrypointLifecycle,
  type BackendAPI,
  type BackendEntrypointFunc,
} from '@aptre/bldr-sdk'

export { createPluginEntrypoint } from '../../plugin/compiler/js/entrypoint.js'

// startWorkerPluginEntrypoint waits for startup and observes process-lifetime failure.
export async function startWorkerPluginEntrypoint(
  entrypoint: BackendEntrypointFunc,
  api: BackendAPI,
  abortSignal: AbortSignal,
  runtimeWasmEnv: Record<string, string> | undefined,
  reportRuntimeFailure: (err: unknown) => void,
): Promise<void> {
  // Start the plugin and wait for its existing readiness contract.
  const result = entrypoint(api, abortSignal, runtimeWasmEnv)
  if (!isBackendEntrypointLifecycle(result)) {
    await result
    return
  }

  // Observe failures for the full lifetime without blocking startup on shutdown.
  void Promise.resolve(result.done).catch(reportRuntimeFailure)
  await result.startup

  // A started plugin that serves no RPC answers every call as not found, so
  // host probes such as Activation.Check learn the method is absent instead
  // of waiting for a handler that never comes.
  if (!api.handleStreamCtr.value) {
    const server = new Server(createMux().lookupMethod)
    api.handleStreamCtr.set(async (stream) => {
      server.handlePacketStream(stream)
    })
  }
}
