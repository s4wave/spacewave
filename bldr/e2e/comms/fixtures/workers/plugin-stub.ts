// plugin-stub.ts - Minimal plugin module for DedicatedWorker hosting test.
//
// Exports a default function (the plugin main). When called, posts a
// "started" message back to the host.

export default function main(signal: AbortSignal) {
  if (!signal.aborted) {
    self.postMessage({ type: 'plugin-started' })
  }
}
