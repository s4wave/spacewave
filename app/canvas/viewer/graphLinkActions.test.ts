import { describe, expect, it, vi } from 'vitest'

import { deleteCanvasGraphLink } from './graphLinkActions.js'
import type { EphemeralEdge } from '../types.js'

function makeEdge(overrides: Partial<EphemeralEdge> = {}): EphemeralEdge {
  return {
    renderKey: 'edge',
    subject: '<objects/a>',
    predicate: '<relatedTo>',
    object: '<objects/b>',
    label: 'main',
    sourceNodeId: 'node-a',
    sourceObjectKey: 'objects/a',
    sourceGroupKey: 'node-a',
    sourceGroupIndex: 0,
    sourceGroupOffset: 0,
    outgoingTruncated: false,
    incomingTruncated: false,
    hiddenCount: 0,
    direction: 'out',
    linkedObjectKey: 'objects/b',
    linkedObjectLabel: 'Object B',
    hideable: true,
    userRemovable: true,
    protected: false,
    ownerManaged: false,
    ...overrides,
  }
}

describe('deleteCanvasGraphLink', () => {
  it('deletes policy-approved graph links and refreshes after success', async () => {
    const deleteGraphQuad = vi.fn().mockResolvedValue(undefined)
    const onDeleted = vi.fn()
    const onError = vi.fn()
    const link = makeEdge()

    await deleteCanvasGraphLink({
      link,
      world: { deleteGraphQuad },
      onError,
      onDeleted,
    })

    expect(deleteGraphQuad).toHaveBeenCalledWith(
      '<objects/a>',
      '<relatedTo>',
      '<objects/b>',
      'main',
    )
    expect(onDeleted).toHaveBeenCalledOnce()
    expect(onError).not.toHaveBeenCalled()
  })

  it('does not delete policy-blocked graph links', async () => {
    const deleteGraphQuad = vi.fn().mockResolvedValue(undefined)
    const onDeleted = vi.fn()
    const onError = vi.fn()

    await deleteCanvasGraphLink({
      link: makeEdge({ userRemovable: false, ownerManaged: true }),
      world: { deleteGraphQuad },
      onError,
      onDeleted,
    })

    expect(deleteGraphQuad).not.toHaveBeenCalled()
    expect(onDeleted).not.toHaveBeenCalled()
    expect(onError).toHaveBeenCalledWith(
      'This graph link is owned by its object type and cannot be deleted here.',
    )
  })

  it('does not refresh when deletion fails', async () => {
    const deleteGraphQuad = vi.fn().mockRejectedValue(new Error('boom'))
    const onDeleted = vi.fn()
    const onError = vi.fn()

    await deleteCanvasGraphLink({
      link: makeEdge(),
      world: { deleteGraphQuad },
      onError,
      onDeleted,
    })

    expect(onDeleted).not.toHaveBeenCalled()
    expect(onError).toHaveBeenCalledWith(
      'Deleting the graph link failed. The link was not removed.',
    )
  })
})
