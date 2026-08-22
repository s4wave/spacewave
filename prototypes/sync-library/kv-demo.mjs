// ElectricSQL-style subscribe/query/mutate flow over the compiled world KV API.
import * as context from '@goscript/context/index.js'
import {
  KvOpen, KvPut, KvGet, KvList, KvWatch, KvDelete, KvStopWatches, KvClose,
} from '@goscript/github.com/s4wave/spacewave/prototypes/sync-library/lean/index.js'

await KvOpen(context.Background())

const snapshots = []
KvWatch('task/', (snapshot) => snapshots.push(JSON.parse(snapshot)))

await KvPut('task/1', JSON.stringify({ title: 'write compiler', done: false }))
await KvPut('task/2', JSON.stringify({ title: 'ship library', done: false }))

await new Promise((r) => setTimeout(r, 250))
console.log('live snapshots:', snapshots.length)
console.log('latest:', snapshots[snapshots.length - 1])

console.log('get:', await KvGet('task/1'))
console.log('list:', await KvList('task/'))

await KvDelete('task/1')
await new Promise((r) => setTimeout(r, 150))
console.log('after delete:', snapshots[snapshots.length - 1])

KvStopWatches()
KvClose()
console.log('kv-demo OK')
// TODO: world teardown leaves a live handle; until found, exit explicitly.
process.exit(0)
