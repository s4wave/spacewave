import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { CreateGitRepoWizardOp } from '@s4wave/core/git/git.pb.js'
import { SetSpaceIndexPathOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { CreateVmV86Op } from '@s4wave/sdk/vm/v86.pb.js'
import { CreateWasiVmOp } from '@s4wave/sdk/vm/wasivm.pb.js'
import { CREATE_WASIVM_OP_ID } from '@s4wave/sdk/vm/wasivm.js'
import { CREATE_VM_V86_OP_ID } from '@s4wave/sdk/vm/vmv86.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { WorldErrorCode } from '@s4wave/sdk/world/world.pb.js'
import type { WorldMutation } from '@s4wave/sdk/world/world.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// WizardWorldState is what a wizard test World holds: the object keys, the
// Space index, and the create ops applied, in order. A create op whose key
// the World decodes adds its object to objects.
export interface WizardWorldState {
  objects: Set<string>
  indexPath: string
  creates: string[]
}

// createdKeys decodes the object key each create op the wizards apply writes,
// so the test World holds the object and refuses a second create at its key.
const createdKeys: Record<string, (opData: Uint8Array) => string> = {
  [CREATE_VM_V86_OP_ID]: (d) => CreateVmV86Op.fromBinary(d).objectKey ?? '',
  [CREATE_WASIVM_OP_ID]: (d) => CreateWasiVmOp.fromBinary(d).objectKey ?? '',
  'spacewave/git/repo/create': (d) =>
    CreateGitRepoWizardOp.fromBinary(d).objectKey ?? '',
  'spacewave/forge/job/create': (d) =>
    ForgeJobCreateOp.fromBinary(d).jobKey ?? '',
  'spacewave/forge/task/create': (d) =>
    ForgeTaskCreateOp.fromBinary(d).taskKey ?? '',
}

// applyCreate stages the object a create op writes. Like the World, it
// refuses a create at a key another object holds.
function applyCreate(
  state: WizardWorldState,
  opId: string,
  opData: Uint8Array,
) {
  const key = createdKeys[opId]?.(opData)
  if (key) {
    if (state.objects.has(key)) throw new Error('object already exists')
    state.objects.add(key)
  }
  state.creates.push(opId)
}

// WizardTestWorld is a Space World held in memory for wizard Finish tests.
// A batch stages a copy of the state and publishes it only when every
// mutation applied, as the World engine does. calls lists each call in order:
// 'batch', then each op id or 'delete', then 'commit' when the batch lands.
// opData and senders hold each op's data and sender by op id, last write
// wins. failOn names the one call that throws: an op id, 'delete' or
// 'commit'. A batch waits for commitGate, once its mutations applied, when
// one is set.
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
// getObject finds the committed objects; base supplies the other reads code
// outside the transaction makes, such as the listing that picks a free key.
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

  // applyMutation stages one batch mutation, returning the code the World
  // refuses it with, if any.
  const applyMutation = (staged: WizardWorldState, m: WorldMutation) => {
    switch (m.mutation?.case) {
      case 'op': {
        const {
          opTypeId = '',
          opData = new Uint8Array(),
          opSender = '',
        } = m.mutation.value
        fail(opTypeId)
        t.opData.set(opTypeId, opData)
        t.senders.set(opTypeId, opSender)
        if (opTypeId !== SET_SPACE_INDEX_PATH_OP_ID) {
          applyCreate(staged, opTypeId, opData)
          return
        }
        // The World refuses, without error, an index move that expects an
        // index another writer has since changed; a move that expects
        // nothing is unconditional.
        const op = SetSpaceIndexPathOp.fromBinary(opData)
        const expected = op.expectedIndexPath
        if (expected === undefined || expected === staged.indexPath) {
          staged.indexPath = op.indexPath ?? ''
        }
        return
      }
      case 'deleteObject':
        fail('delete')
        if (!staged.objects.delete(m.mutation.value.objectKey ?? '')) {
          return WorldErrorCode.OBJECT_NOT_FOUND
        }
        return
      default:
        throw new Error('unexpected mutation ' + m.mutation?.case)
    }
  }

  // executeWorldBatch stages a copy, applies each mutation, and publishes the
  // copy only when none was refused.
  const executeWorldBatch = async (mutations: WorldMutation[]) => {
    const staged: WizardWorldState = {
      objects: new Set(t.state.objects),
      indexPath: t.state.indexPath,
      creates: [...t.state.creates],
    }
    t.calls.push('batch')
    for (const [failedIndex, m] of mutations.entries()) {
      const errorCode = applyMutation(staged, m)
      if (errorCode) return { failedIndex, errorCode }
    }
    await t.commitGate
    fail('commit')
    t.state = staged
    return { seqno: 1n }
  }

  // A write on the World itself commits on its own, as a one-op
  // transaction does.
  t.world = {
    getObject: (key: string) =>
      Promise.resolve(
        t.state.objects.has(key) ? { [Symbol.dispose]: () => {} } : null,
      ),
    ...base,
    getEngine: () => ({ executeWorldBatch }),
    applyWorldOp: (opId: string, opData: Uint8Array, sender: string) => {
      // Record the op and publish what it writes.
      fail(opId)
      t.opData.set(opId, opData)
      t.senders.set(opId, sender)
      applyCreate(t.state, opId, opData)
      return Promise.resolve({ seqno: 1n, sysErr: false })
    },
  } as unknown as EngineWorldState
  return t
}
