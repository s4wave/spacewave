import { describe, expect, it } from 'vitest'

import { WorldOperationRejection } from '@s4wave/sdk/world/errors.js'
import type { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import {
  applyNotebookEdit,
  NOTEBOOK_CONFLICT_CODE,
  NOTEBOOK_ORDER_CODE,
} from './notebook-edit.js'
import {
  Notebook,
  type NotebookEditOp,
  type NotebookSource,
} from './proto/notebook.pb.js'

const a = { name: 'A', ref: 'a/-/' }
const b = { name: 'B', ref: 'b/-/' }
const c = { name: 'C', ref: 'c/-/' }

// memoryWorld holds one Notebook object in a content-addressed block map and
// lists every call the op makes on the World state. edit applies an op the
// way the World writer does: one at a time, each seeing the last one's write.
function memoryWorld(sources: NotebookSource[]) {
  const blocks = new Map<string, Uint8Array>()
  const calls: string[] = []
  let next = 0
  const put = (data: Uint8Array) => {
    const key = String(++next)
    blocks.set(key, data)
    return { hash: { hash: new TextEncoder().encode(key) } }
  }
  const get = (ref?: ObjectRef) =>
    blocks.get(new TextDecoder().decode(ref?.rootRef?.hash?.hash))
  let root: ObjectRef = {
    bucketId: 'notes',
    rootRef: put(Notebook.toBinary({ name: 'Notes', sources })),
  }

  const object = {
    getRootRef: () => {
      calls.push('getRootRef')
      return Promise.resolve({ rootRef: root })
    },
    accessWorldState: (ref: ObjectRef) => {
      calls.push('accessWorldState')
      return Promise.resolve({
        getBlock: () => {
          calls.push('getBlock')
          const data = get(ref)
          return Promise.resolve({ found: !!data, data })
        },
        putBlock: (req: { data: Uint8Array }) => {
          calls.push('putBlock')
          return Promise.resolve({ ref: put(req.data) })
        },
        [Symbol.dispose]: () => {},
      })
    },
    setRootRef: (ref: ObjectRef) => {
      calls.push('setRootRef')
      root = ref
      return Promise.resolve({})
    },
    [Symbol.dispose]: () => {},
  }
  const ws = {
    getObject: (key: string) => {
      calls.push('getObject')
      return Promise.resolve(key === 'notebook' ? object : null)
    },
  } as unknown as IWorldState

  let writer = Promise.resolve()
  const edit = (opEdit: NotebookEditOp['edit']) => {
    const run = writer.then(() =>
      applyNotebookEdit(ws, { objectKey: 'notebook', edit: opEdit }),
    )
    writer = run.catch(() => {})
    return run
  }
  const read = () => Notebook.fromBinary(get(root)!).sources ?? []
  return { calls, edit, read }
}

// rejection returns the code an edit was refused with.
async function rejection(edit: Promise<void>): Promise<string> {
  const err: unknown = await edit.then(
    () => undefined,
    (e: unknown) => e,
  )
  expect(err).toBeInstanceOf(WorldOperationRejection)
  return (err as WorldOperationRejection).code
}

describe('applyNotebookEdit', () => {
  it('reads and writes with six World calls and nothing else', async () => {
    const w = memoryWorld([a])
    await w.edit({ case: 'add', value: { source: b } })
    expect(w.calls).toEqual([
      'getObject',
      'getRootRef',
      'accessWorldState',
      'getBlock',
      'putBlock',
      'setRootRef',
    ])
    expect(w.read()).toEqual([a, b])
  })

  it('refuses a remove at an index that now holds another source', async () => {
    const w = memoryWorld([a, b, c])
    // Another writer removes A, so index 1 now holds C, not the B the
    // caller saw there.
    await w.edit({ case: 'remove', value: { index: 0, source: a } })
    w.calls.length = 0
    const code = await rejection(
      w.edit({ case: 'remove', value: { index: 1, source: b } }),
    )
    expect(code).toBe(NOTEBOOK_CONFLICT_CODE)
    expect(w.read()).toEqual([b, c])
    expect(w.calls).not.toContain('putBlock')
    expect(w.calls).not.toContain('setRootRef')

    // An index past the end is the same conflict: the source moved away.
    const past = await rejection(
      w.edit({ case: 'remove', value: { index: 2, source: c } }),
    )
    expect(past).toBe(NOTEBOOK_CONFLICT_CODE)
  })

  it('refuses a reorder against a list another writer changed', async () => {
    const w = memoryWorld([a, b])
    await w.edit({ case: 'add', value: { source: c } })
    const code = await rejection(
      w.edit({ case: 'reorder', value: { order: [1, 0], sources: [a, b] } }),
    )
    expect(code).toBe(NOTEBOOK_CONFLICT_CODE)
    expect(w.read()).toEqual([a, b, c])

    // The same reorder against the current list applies.
    await w.edit({
      case: 'reorder',
      value: { order: [2, 0, 1], sources: [a, b, c] },
    })
    expect(w.read()).toEqual([c, a, b])
  })

  it('refuses an order that is not a permutation before reading', async () => {
    const w = memoryWorld([a, b])
    const code = await rejection(
      w.edit({ case: 'reorder', value: { order: [0, 0], sources: [a, b] } }),
    )
    expect(code).toBe(NOTEBOOK_ORDER_CODE)
    expect(w.calls).toEqual([])
  })

  it('keeps both sources when two adds race', async () => {
    const w = memoryWorld([a])
    await Promise.all([
      w.edit({ case: 'add', value: { source: b } }),
      w.edit({
        case: 'add',
        value: { source: { name: ' C ', ref: ' c/-/ ' } },
      }),
    ])
    expect(w.read()).toEqual([a, b, c])
  })
})
