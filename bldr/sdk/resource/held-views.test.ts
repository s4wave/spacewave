import { expect, it } from 'vitest'

import { HeldViews, type Versioned } from './held-views.js'

/** TestView is a synthetic immutable view used only by this run-through. */
interface TestView extends Versioned {
  rows?: string[]
}

it('resumes whole views, applies deltas and rejects a missing base', async () => {
  // The first watch holds its full result and supplies its version at reopen.
  const held = new HeldViews()
  const versions: string[] = []
  const controller = new AbortController()
  async function read(messages: TestView[], cache = held) {
    const result: TestView[] = []
    const start = async function* (version: string) {
      versions.push(version)
      yield* messages
    }
    for await (const view of cache.resume(
      'rows', start, controller.signal, undefined,
      (previous, change) => ({ rows: [...(previous.rows ?? []), ...(change.rows ?? [])] }),
    )) result.push(view)
    return result
  }
  expect(await read([{ rows: ['a'], view: { version: 'v1' } }])).toEqual([
    { rows: ['a'], view: { version: 'v1' } },
  ])

  // A held version resumes unchanged, then merges a change into a whole view.
  expect(await read([
    { view: { version: 'v1', unchanged: true, revision: 2n } },
    { rows: ['b'], view: { version: 'v2', changes: true, revision: 3n } },
  ])).toEqual([
    { rows: ['a'], view: { version: 'v1', revision: 2n } },
    { rows: ['a', 'b'], view: { version: 'v2', revision: 3n } },
  ])
  expect(versions).toEqual(['', 'v1'])

  // A new authorization scope cannot resume a previous principal's view.
  const other = new HeldViews()
  await expect(read([{ view: { version: 'v2', unchanged: true } }], other)).rejects.toThrow()
  await expect(read([{ rows: ['c'], view: { version: 'v3', changes: true } }], other)).rejects.toThrow()
  expect(await read([{ rows: ['c'], view: { version: 'v3' } }], other)).toEqual([
    { rows: ['c'], view: { version: 'v3' } },
  ])
})
