// plugin-stub.ts - Minimal plugin module for DedicatedWorker hosting test.
//
// Exports a default function (the plugin main). When called, posts a
// "started" message back to the host and listens for one bus message.

export default function main(
  busEndpoint: { read: () => Promise<{ sourceId: number; data: Uint8Array } | null> },
  signal: AbortSignal,
) {
  self.postMessage({ type: 'plugin-started' })

  // Read one message from bus and report it.
  busEndpoint
    .read()
    .then((msg) => {
      if (msg && !signal.aborted) {
        self.postMessage({
          type: 'plugin-received',
          sourceId: msg.sourceId,
          data: Array.from(msg.data),
        })
      }
    })
    .catch(() => {})
}
