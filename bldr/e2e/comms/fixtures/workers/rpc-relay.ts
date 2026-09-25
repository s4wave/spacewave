// rpc-relay stands in for an engine's device worker under topology T2: each
// request carries the call's argument bytes, and each response carries a
// result of the requested length, so a round trip costs what an RPC hop does.

// Request is one relayed call.
interface Request {
  id: number
  send: Uint8Array
  recv: number
}

self.addEventListener('message', (ev: MessageEvent<Request>) => {
  const { id, recv } = ev.data
  self.postMessage({ id, data: new Uint8Array(recv) })
})
