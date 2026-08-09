import { describe, expect, it, vi } from 'vitest'

import { buildSpaceObjectActionTargets } from '@s4wave/web/space/object-tree.js'
import { createSpaceObjectNavigationActions } from './space-object-navigation-actions.js'

describe('space object navigation actions', () => {
  it('builds visible object targets from the Space object tree policy', () => {
    const targets = buildSpaceObjectActionTargets([
      { objectKey: 'settings', objectType: 'space/settings' },
      { objectKey: 'files', objectType: 'unixfs/fs-node' },
      { objectKey: 'object-layout/main', objectType: 'alpha/object-layout' },
    ])

    expect(targets.map((target) => target.objectKey)).toEqual([
      'files',
      'object-layout/main',
    ])
    expect(targets.map((target) => target.label)).toEqual(['files', 'main'])
  })

  it('creates only navigation-focused bottom-bar actions', async () => {
    const openDetails = vi.fn()
    const openObject = vi.fn()
    const switchObjectHere = vi.fn()
    const targets = buildSpaceObjectActionTargets([
      { objectKey: 'files', objectType: 'unixfs/fs-node' },
      { objectKey: 'canvas-1', objectType: 'canvas' },
    ])

    const actions = createSpaceObjectNavigationActions({
      targets,
      currentObjectKey: 'files',
      openDetails,
      openObject,
      switchObjectHere,
    })

    expect(actions.map((action) => action.id)).toEqual([
      'open-details',
      'browse-objects',
      'switch-object-here',
    ])
    const labels = JSON.stringify(actions)
    expect(labels).toContain('Open Details')
    expect(labels).toContain('Browse Objects')
    expect(labels).toContain('Switch Object Here')
    expect(labels).not.toContain('Delete')
    expect(labels).not.toContain('Rename')
    expect(labels).not.toContain('Set as Index')

    const detailsAction = actions[0]
    expect(detailsAction.type).toBe('action')
    if (detailsAction.type !== 'action') return
    const openPrimaryOverlay = vi.fn()
    await detailsAction.onSelect({
      itemId: 'sharedObject',
      openKind: 'mouse',
      closeMenu: vi.fn(),
      openPrimaryOverlay,
    })
    expect(openPrimaryOverlay).toHaveBeenCalledTimes(1)
    expect(openDetails).toHaveBeenCalledTimes(1)

    const browseGroup = actions[1]
    expect(browseGroup.type).toBe('group')
    if (browseGroup.type !== 'group') return
    const currentAction = browseGroup.items.find(
      (item) => item.type === 'action' && item.id === 'open:files',
    )
    if (!currentAction || currentAction.type !== 'action') {
      throw new Error('expected current object open action')
    }
    expect(currentAction.disabled).toBe(true)

    const switchGroup = actions[2]
    expect(switchGroup.type).toBe('group')
    if (switchGroup.type !== 'group') return
    const switchAction = switchGroup.items[1]
    expect(switchAction.type).toBe('action')
    if (switchAction.type !== 'action') return
    await switchAction.onSelect({
      itemId: 'sharedObject',
      openKind: 'mouse',
      closeMenu: vi.fn(),
      openPrimaryOverlay: vi.fn(),
    })
    expect(switchObjectHere).toHaveBeenCalledWith(targets[1])
  })
})
