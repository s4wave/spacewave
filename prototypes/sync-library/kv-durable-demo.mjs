// Durable persistence proof: two open/close cycles over one snapshot file.
import * as context from '@goscript/context/index.js'
import { KvOpenDurable, KvPut, KvGet, KvClose } from '@goscript/github.com/s4wave/spacewave/prototypes/sync-library/lean/index.js'

const ctx = context.Background()
const dir = '/tmp/sync-kv-durable-demo'

{
  const err = await KvOpenDurable(ctx, dir)
  if (err) throw new Error(`open: ${err.message}`)
  await KvPut('durable/a', 'one')
  await KvPut('durable/b', 'two')
  console.log('cycle 1: wrote durable/a=one, durable/b=two')
  KvClose()
}

{
  const err = await KvOpenDurable(ctx, dir)
  if (err) throw new Error(`reopen: ${err.message}`)
  const [a] = await KvGet('durable/a')
  const [b] = await KvGet('durable/b')
  console.log(`cycle 2 readback: durable/a=${JSON.stringify(a)} durable/b=${JSON.stringify(b)}`)
  if (a !== 'one' || b !== 'two') throw new Error('durable readback mismatch')
  KvClose()
}
console.log('durable demo OK')
process.exit(0)
