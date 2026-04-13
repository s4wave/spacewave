import { PluginWorker } from '../../../../web/runtime/plugin-worker.js'

declare const self: DedicatedWorkerGlobalScope

function readMode(): string {
  return new URL(self.location.href).searchParams.get('mode') ?? 'import-fail'
}

new PluginWorker(
  self,
  async () => {
    const mode = readMode()
    if (mode === 'idle') {
      return
    }
    if (mode === 'import-fail') {
      const missingModule = '/workers/does-not-exist.js'
      await import(/* @vite-ignore */ missingModule)
      return
    }
    throw new Error(`unknown startup fixture mode: ${mode}`)
  },
  null,
)

self.postMessage({ type: 'booted' })
