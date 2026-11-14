import { SetSpaceIndexPathOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// WizardWorldState is what a wizard test World holds: the object keys, the
// Space index, and the create ops applied, in order.
export interface WizardWorldState {
  objects: Set<string>
  indexPath: string
  creates: string[]
}

// WizardTestWorld is a Space World held in memory for wizard Finish tests.
// Writes go through write transactions that stage a copy of the state and
// publish it only on commit, as the World engine does. calls lists each call
// in order, and opData and senders each op's data and sender by op id, last
// write wins. failOn names the one call that throws: an op id, 'delete' or
// 'commit'. A commit waits for commitGate when one is set; given a signal
// that aborted meanwhile, it lands and then rejects, as a commit call whose
// caller gave up after sending it does.
export interface WizardTestWorld {
  world: EngineWorldState
  state: WizardWorldState
  calls: string[]
  opData: Map<string, Uint8Array>
  senders: Map<string, string>
  failOn?: string
  commitGate?: Promise<void>
}

// wizardTestWorld builds a test World holding the objects and index given.
// base supplies the reads code outside the transaction makes, such as the
// listing that picks a free key.
export function wizardTestWorld(
  objects: string[],
  indexPath: string,
  base: Partial<IWorldState> = {},
): WizardTestWorld {
  // The World is set below, once its calls can close over t.
  const t: WizardTestWorld = {
    world: undefined as never,
    state: { objects: new Set(objects), indexPath, creates: [] },
    calls: [],
    opData: new Map(),
    senders: new Map(),
  }

  // fail records a call and throws when the test chose it.
  const fail = (call: string) => {
    t.calls.push(call)
    if (t.failOn === call) throw new Error(`${call} failed`)
  }

  // newTransaction stages a copy that commit publishes.
  const newTransaction = () => {
    const staged: WizardWorldState = {
      objects: new Set(t.state.objects),
      indexPath: t.state.indexPath,
      creates: [...t.state.creates],
    }
    t.calls.push('open')
    const tx = {
      applyWorldOp: (opId: string, opData: Uint8Array, sender: string) => {
        // Record the op, then stage what it writes.
        fail(opId)
        t.opData.set(opId, opData)
        t.senders.set(opId, sender)
        if (opId !== SET_SPACE_INDEX_PATH_OP_ID) {
          staged.creates.push(opId)
        } else {
          // The World refuses, without error, an index move that expects an
          // index another writer has since changed.
          const op = SetSpaceIndexPathOp.fromBinary(opData)
          if ((op.expectedIndexPath ?? '') === staged.indexPath) {
            staged.indexPath = op.indexPath ?? ''
          }
        }
        return Promise.resolve({ seqno: 1n, sysErr: false })
      },
      deleteObject: (objectKey: string) => {
        fail('delete')
        return Promise.resolve({ deleted: staged.objects.delete(objectKey) })
      },
      commit: async (signal?: AbortSignal) => {
        await t.commitGate
        fail('commit')
        t.state = staged
        signal?.throwIfAborted()
      },
      discard: () => {
        t.calls.push('discard')
        return Promise.resolve()
      },
      release: () => {
        t.calls.push('release')
      },
    }
    return Promise.resolve(tx)
  }

  // A write on the World itself commits on its own, as a one-op
  // transaction does.
  t.world = {
    ...base,
    getEngine: () => ({ newTransaction }),
    applyWorldOp: (opId: string, opData: Uint8Array, sender: string) => {
      // Record the op and publish what it writes.
      fail(opId)
      t.opData.set(opId, opData)
      t.senders.set(opId, sender)
      t.state.creates.push(opId)
      return Promise.resolve({ seqno: 1n, sysErr: false })
    },
  } as unknown as EngineWorldState
  return t
}
