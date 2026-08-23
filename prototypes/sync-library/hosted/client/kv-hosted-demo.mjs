// Hosted transport proof: compiled JS client over WebSocket SRPC to a Go
// server hosting the authoritative world. The server writes a sentinel key
// three seconds after startup; the client lists all keys after waiting
// past that point and asserts cross-process visibility.
import * as context from '@goscript/context/index.js'
import { KvOpenHosted, KvList } from '@goscript/github.com/s4wave/spacewave/prototypes/sync-library/lean/index.js'

const ctx = context.Background()
const err = await KvOpenHosted(ctx, 'ws://127.0.0.1:8907/ws')
if (err) throw new Error(`open: ${err.message}`)

await new Promise((r) => setTimeout(r, 5000))

const [listJson, listErr] = await KvList('')
if (listErr) throw new Error(`list: ${listErr.message || String(listErr)}`)
const entries = JSON.parse(listJson)
console.log('hosted keys:', entries.map((e) => e.key))
const found = entries.find((e) => e.key === 'server/hello')
if (!found) throw new Error('server/hello not visible to client')
console.log('CROSS-PROCESS SYNC OK:', found.key, '=', found.value)
process.exit(0)
