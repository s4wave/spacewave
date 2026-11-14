import { describe, expect, it, vi } from 'vitest'

import { SetSpaceIndexPathOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { CREATE_VM_V86_OP_ID } from '@s4wave/sdk/vm/vmv86.js'

import { runVmCreation, type VmCreationRequest } from './v86-creation.js'

describe('runVmCreation', () => {
  it('creates the VM, moves an index naming the wizard to it, then deletes the wizard', async () => {
    // A fake World records each op and delete in the order they arrive.
    const calls: string[] = []
    const applyWorldOp = vi.fn((opId: string, opData: Uint8Array) => {
      calls.push(opId)
      if (opId === SET_SPACE_INDEX_PATH_OP_ID) {
        const op = SetSpaceIndexPathOp.fromBinary(opData)
        calls.push(`index ${op.indexPath} from ${op.expectedIndexPath ?? ''}`)
      }
      return Promise.resolve({ seqno: 1n, sysErr: false })
    })
    const deleteObject = vi.fn((objectKey: string) => {
      calls.push(`delete ${objectKey}`)
      return Promise.resolve({ deleted: true })
    })
    const world = { applyWorldOp, deleteObject } as never
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
      spaceIndexPath: 'wizard/vm-1',
    }

    // Drain the stream; its last stage is ready.
    const stages: string[] = []
    const signal = new AbortController().signal
    for await (const p of runVmCreation(world, request, signal)) {
      stages.push(p.stage)
    }
    expect(stages).toEqual(['creating', 'ready'])

    // The VM exists before the index names it, and the wizard goes last.
    expect(calls).toEqual([
      CREATE_VM_V86_OP_ID,
      SET_SPACE_INDEX_PATH_OP_ID,
      'index vm/v86/debian from wizard/vm-1',
      'delete wizard/vm-1',
    ])
  })
})
