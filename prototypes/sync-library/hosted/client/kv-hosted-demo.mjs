// Hosted transport proof: compiled JS client over WebSocket SRPC to a Go
// server process. Subscribes first, then observes the server-originated
// write, then writes from the client side and reads it back.
import * as context from '@goscript/context/index.js'
import {
  KvOpenHosted, KvWatch, KvPut, KvGet, KvList, KvClose,
} from '@goscript/github.com/s4wave/spacewave/prototypes/sync-library/lean/index.js'

const ctx = context.Background()
await KvOpenHosted(ctx, 'ws://127.0.0.1:8900/ws')
console.log('connected to hosted world')

const snapshots = []
KvWatch('server/', (snapshot) => snapshots.push(JSON.parse(snapshot)))

// wait for the server's delayed write to arrive in a snapshot
const deadline = Date.now() + 15000
let observed = false
while (Date.now() < deadline) {
  const hit = snapshots.find((s) => s.some((e) => e.key === 'server/hello'))
  if (hit) {
    const entry = hit.find((e) => e.key === 'server/hello')
    console.log('server write observed via WatchPrefix:', JSON.stringify(entry))
    observed = true
    break
  }
  await new Promise((r) => setTimeout(r, 200))
}
if (!observed) throw new Error('never observed server/hello from the other process')

// client-side mutation round-trip against the hosted authoritative world
await KvPut('client/note', 'written-from-client')
const got = await KvGet('client/note')
console.log('client write readback:', got)
const list = await KvList('')
console.log('total keys visible:', JSON.parse(list[0]).length)

KvClose()
console.log('hosted demo OK')
process.exit(0)
