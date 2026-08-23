// Hosted transport proof: compiled JS client over WebSocket SRPC to a Go
// server hosting the authoritative world. Subscribes via WatchPrefix,
// observes the server-originated write cross-process, then writes from
// the client side and reads back.
import * as context from '@goscript/context/index.js'
import { KvOpenHosted, KvWatch, KvPut, KvGet, KvList } from '@goscript/github.com/s4wave/spacewave/prototypes/sync-library/lean/index.js'

const ctx = context.Background()
const openErr = await KvOpenHosted(ctx, 'ws://127.0.0.1:8907/ws')
if (openErr) throw new Error(`open: ${openErr.Error()}`)
console.log('connected to hosted world')

const snapshots = []
KvWatch('server/', (snapshot) => snapshots.push(JSON.parse(snapshot)))

const deadline = Date.now() + 15000
let observed = false
while (Date.now() < deadline) {
  const hit = snapshots.find((s) => s.some((e) => e.key === 'server/hello'))
  if (hit) {
    const entry = hit.find((e) => e.key === 'server/hello')
    console.log('CROSS-PROCESS WRITE OBSERVED:', JSON.stringify(entry))
    observed = true
    break
  }
  await new Promise((r) => setTimeout(r, 200))
}
if (!observed) throw new Error('server/hello never appeared')

await KvPut('client/note', 'written-from-client')
const [got] = await KvGet('client/note')
console.log('client write readback:', JSON.stringify(got))
process.exit(0)
