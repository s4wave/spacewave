import { describe, expect, it } from 'vitest'

import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { CREATE_VM_V86_OP_ID } from '@s4wave/sdk/vm/vmv86.js'

import { wizardTestWorld } from '../wizard/wizard-test-world.js'
import {
  createVm,
  runVmCreation,
  vmCreationReducer,
  type VmCreationEvent,
  type VmCreationRequest,
  type VmCreationState,
} from './v86-creation.js'

const request: VmCreationRequest = {
  linkCdnImage: false,
  cdnId: '',
  cdnSourceObjectKey: '',
  imageObjectKey: 'vm/image/debian',
  vmKey: 'vm/v86/debian',
  vmName: 'Debian',
  memoryMb: 256,
  vgaMemoryMb: 8,
  networking: false,
  sessionPeerId: 'peer-1',
  wizardObjectKey: 'wizard/vm-1',
}

// drain runs the creation stream to its end and returns the stages.
async function drain(t: ReturnType<typeof wizardTestWorld>) {
  const stages: string[] = []
  const signal = new AbortController().signal
  for await (const p of runVmCreation(t.world, request, signal)) {
    stages.push(p.stage)
  }
  return stages
}

// run starts the creation from state and runs it to its end through the
// reducer, as the viewer does, and returns the state it ends in.
async function run(
  t: ReturnType<typeof wizardTestWorld>,
  state: VmCreationState = { status: 'idle' },
) {
  let creation = vmCreationReducer(state, { type: 'start', request })
  const dispatch = (event: VmCreationEvent) => {
    creation = vmCreationReducer(creation, event)
  }
  await createVm(t.world, request, dispatch, new AbortController().signal)
  return creation
}

describe('runVmCreation', () => {
  it('creates the VM, moves the index and deletes the wizard in one batch', async () => {
    const t = wizardTestWorld(['wizard/vm-1'], 'wizard/vm-1')
    expect(await drain(t)).toEqual(['creating', 'ready'])

    // The VM create, the index move and the delete commit together.
    expect(t.calls).toEqual([
      'batch',
      CREATE_VM_V86_OP_ID,
      SET_SPACE_INDEX_PATH_OP_ID,
      'delete',
      'commit',
    ])
    expect(t.state).toEqual({
      objects: new Set(['vm/v86/debian']),
      indexPath: 'vm/v86/debian',
      creates: [CREATE_VM_V86_OP_ID],
    })
  })

  it('writes nothing when the wizard delete fails, so a retry with the same key creates once', async () => {
    // The delete throws after the create staged.
    const t = wizardTestWorld(['wizard/vm-1'], 'wizard/vm-1')
    t.failOn = 'delete'
    await expect(drain(t)).rejects.toThrow('delete failed')

    // No VM, the index still names the wizard, and the wizard is there.
    expect(t.state).toEqual({
      objects: new Set(['wizard/vm-1']),
      indexPath: 'wizard/vm-1',
      creates: [],
    })

    // The retry reruns the stream with the same request and creates one VM.
    t.failOn = undefined
    expect(await drain(t)).toEqual(['creating', 'ready'])
    expect(t.state.creates).toEqual([CREATE_VM_V86_OP_ID])
  })

  it('ends the run as done, and done is terminal', async () => {
    const t = wizardTestWorld(['wizard/vm-1'], 'wizard/vm-1')
    const creation = await run(t)
    expect(creation).toEqual({ status: 'done', request })

    // No event reruns the creation or moves it out of done.
    for (const event of [
      { type: 'start', request },
      { type: 'stage', stage: 'creating' },
      { type: 'fail', error: 'object already exists' },
      { type: 'refuse' },
    ] satisfies VmCreationEvent[]) {
      expect(vmCreationReducer(creation, event)).toBe(creation)
    }
    expect(t.state.creates).toEqual([CREATE_VM_V86_OP_ID])
  })

  it('refuses as gone when another writer removed the wizard', async () => {
    const t = wizardTestWorld([], '')
    expect(await run(t)).toEqual({ status: 'gone' })
    expect(t.state.creates).toEqual([])
  })

  it('refuses as gone when a rerun after a landed commit hits the existing VM', async () => {
    // The commit landed but its caller saw a failure, so Retry runs again:
    // the World refuses the VM create before the wizard delete runs.
    const t = wizardTestWorld(['vm/v86/debian'], 'vm/v86/debian')
    const failed: VmCreationState = {
      status: 'failed',
      request,
      stage: 'creating',
      error: 'commit failed',
    }
    expect(await run(t, failed)).toEqual({ status: 'gone' })
    expect(t.state.creates).toEqual([])
  })

  it('offers Retry for a failure while the wizard is still there', async () => {
    const t = wizardTestWorld(['wizard/vm-1'], 'wizard/vm-1')
    t.failOn = 'commit'
    expect(await run(t)).toEqual({
      status: 'failed',
      request,
      stage: 'creating',
      error: 'commit failed',
    })

    // Retry starts the same request again and lands.
    t.failOn = undefined
    expect(
      await run(t, {
        status: 'failed',
        request,
        stage: 'creating',
        error: 'commit failed',
      }),
    ).toEqual({ status: 'done', request })
  })
})
