import { describe, expect, it, vi } from 'vitest'

import type { Engine } from './engine.js'
import { EngineWorldState } from './engine-state.js'
import {
  PluginOpTimeoutError,
  PluginUnavailableError,
  WorldBatchRefusal,
} from './errors.js'
import {
  WorldErrorCode,
  type ExecuteWorldBatchResponse,
  type WorldMutation,
} from './world.pb.js'

// fakeEngine answers each batch with the next response and fails any
// transaction, so a write that opens one fails the test.
function fakeEngine(...responses: ExecuteWorldBatchResponse[]) {
  const executeWorldBatch = vi.fn(async (_mutations: WorldMutation[]) => {
    return responses.shift() ?? { seqno: 1n }
  })
  const newTransaction = vi.fn(() => {
    throw new Error('opened a transaction')
  })
  const engine = { executeWorldBatch, newTransaction } as unknown as Engine
  return { engine, executeWorldBatch, newTransaction }
}

describe('EngineWorldState writes', () => {
  it('sends each write as one batch and opens no transaction', async () => {
    const { engine, executeWorldBatch, newTransaction } = fakeEngine(
      { seqno: 7n },
      { seqno: 8n },
      { seqno: 9n },
      { seqno: 10n, results: [{ rev: 3n }] },
      { seqno: 11n, results: [{ rev: 4n }] },
      { seqno: 12n, results: [{ rev: 5n }] },
    )
    const ws = new EngineWorldState(engine, true)

    const obj = await ws.createObject('a', {})
    await ws.renameObject('a', 'b', { descendants: true })
    expect(await ws.applyWorldOp('op', new Uint8Array(), 'peer')).toEqual({
      seqno: 9n,
      sysErr: false,
    })
    expect(await obj.setRootRef({})).toEqual({ rev: 3n })
    expect(await obj.applyObjectOp('op', new Uint8Array(), 'peer')).toEqual({
      rev: 4n,
    })
    expect(await obj.incrementRev()).toEqual({ rev: 5n })
    await ws.setGraphQuad('<a>', '<p>', '<b>')
    await ws.deleteGraphQuad('<a>', '<p>', '<b>')
    await ws.deleteGraphObject('a')

    const cases = executeWorldBatch.mock.calls.map(([mutations]) => {
      expect(mutations).toHaveLength(1)
      return mutations[0].mutation?.case
    })
    expect(cases).toEqual([
      'createObject',
      'renameObject',
      'op',
      'setRootRef',
      'applyObjectOp',
      'incrementRev',
      'setGraphQuad',
      'deleteGraphQuad',
      'deleteGraphObject',
    ])
    expect(executeWorldBatch.mock.calls[1][0][0].mutation?.value).toEqual({
      oldObjectKey: 'a',
      newObjectKey: 'b',
      descendants: true,
    })
    expect(newTransaction).not.toHaveBeenCalled()
  })

  it('reports an absent object to deleteObject as not deleted', async () => {
    const { engine } = fakeEngine(
      { seqno: 2n },
      { errorCode: WorldErrorCode.OBJECT_NOT_FOUND },
    )
    const ws = new EngineWorldState(engine, true)
    expect(await ws.deleteObject('a')).toEqual({ deleted: true })
    expect(await ws.deleteObject('a')).toEqual({ deleted: false })
  })

  it('throws refusals as typed errors', async () => {
    const { engine } = fakeEngine(
      { seqno: 1n },
      { errorCode: WorldErrorCode.OBJECT_NOT_FOUND },
      { errorCode: WorldErrorCode.PLUGIN_OP_TIMEOUT },
      { errorCode: WorldErrorCode.PLUGIN_UNAVAILABLE },
    )
    const ws = new EngineWorldState(engine, true)
    const obj = await ws.createObject('a', {})
    await expect(obj.incrementRev()).rejects.toBeInstanceOf(WorldBatchRefusal)
    await expect(
      obj.applyObjectOp('op', new Uint8Array(), 'peer'),
    ).rejects.toBeInstanceOf(PluginOpTimeoutError)
    await expect(
      obj.applyObjectOp('op', new Uint8Array(), 'peer'),
    ).rejects.toBeInstanceOf(PluginUnavailableError)
  })

  it('refuses writes on a read-only state without a round trip', async () => {
    const { engine, executeWorldBatch } = fakeEngine()
    const ws = new EngineWorldState(engine, false)
    await expect(ws.createObject('a', {})).rejects.toThrow()
    expect(executeWorldBatch).not.toHaveBeenCalled()
  })
})
