import { describe, expect, it, vi } from 'vitest'

import type { SpaceObjectActionTarget } from '@s4wave/web/space/object-tree.js'
import type { ReplaceTabResponse } from '@s4wave/sdk/layout/layout.pb.js'
import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'

import type { ReplaceCurrentTabRequest } from './TabContext.js'
import {
  hasObjectViewerSwitchOwner,
  switchObjectAtViewerPosition,
} from './object-viewer-space-actions.js'

const target: SpaceObjectActionTarget = {
  objectKey: 'canvas-1',
  objectType: 'canvas',
  label: 'Canvas',
  objectTypeLabel: 'Canvas',
  objectTypeDescription: '',
}

describe('object viewer space actions', () => {
  it('prefers ObjectLayout replace-tab over shell route replacement', async () => {
    const replaceTab = vi.fn<
      (request: ReplaceCurrentTabRequest) => Promise<ReplaceTabResponse>
    >(() => Promise.resolve({}))
    const switchObjectAtCurrentPosition = vi.fn()

    expect(
      hasObjectViewerSwitchOwner({
        tabId: 'layout-tab',
        replaceTab,
        switchObjectAtCurrentPosition,
      }),
    ).toBe(true)

    await switchObjectAtViewerPosition(target, {
      tabId: 'layout-tab',
      replaceTab,
      switchObjectAtCurrentPosition,
    })

    expect(switchObjectAtCurrentPosition).not.toHaveBeenCalled()
    expect(replaceTab).toHaveBeenCalledTimes(1)
    const request = replaceTab.mock.calls[0]?.[0]
    if (!request) throw new Error('expected ReplaceTab request')
    expect(request.tab?.name).toBe('Canvas')
    const payload = ObjectLayoutTab.fromBinary(
      request.tab?.data ?? new Uint8Array(),
    )
    expect(payload.componentId).toBeUndefined()
    expect(payload.path ?? '').toBe('')
    expect(payload.objectInfo?.info).toMatchObject({
      case: 'worldObjectInfo',
      value: {
        objectKey: 'canvas-1',
        objectType: 'canvas',
      },
    })
  })

  it('falls back to the shell position owner outside ObjectLayout', async () => {
    const switchObjectAtCurrentPosition = vi.fn()

    expect(hasObjectViewerSwitchOwner({ switchObjectAtCurrentPosition })).toBe(
      true,
    )

    await switchObjectAtViewerPosition(target, {
      switchObjectAtCurrentPosition,
    })

    expect(switchObjectAtCurrentPosition).toHaveBeenCalledWith({
      objectKey: 'canvas-1',
    })
  })
})
