import type { ReplaceTabResponse } from '@s4wave/sdk/layout/layout.pb.js'
import { createObjectLayoutReplaceTabRequest } from '@s4wave/sdk/layout/world/object-layout.js'
import type { SwitchObjectAtCurrentPositionFunc } from '@s4wave/web/contexts/SpaceContainerContext.js'
import type { SpaceObjectActionTarget } from '@s4wave/web/space/object-tree.js'

import type { ReplaceCurrentTabRequest } from './TabContext.js'

export interface ObjectViewerPositionOwner {
  tabId?: string
  replaceTab?: (
    request: ReplaceCurrentTabRequest,
  ) => Promise<ReplaceTabResponse>
  switchObjectAtCurrentPosition?: SwitchObjectAtCurrentPositionFunc
}

export function hasObjectViewerSwitchOwner(
  owner: ObjectViewerPositionOwner,
): boolean {
  return (
    !!(owner.tabId && owner.replaceTab) || !!owner.switchObjectAtCurrentPosition
  )
}

export async function switchObjectAtViewerPosition(
  target: SpaceObjectActionTarget,
  owner: ObjectViewerPositionOwner,
): Promise<void> {
  if (owner.tabId && owner.replaceTab) {
    await owner.replaceTab(
      createObjectLayoutReplaceTabRequest({
        tabId: owner.tabId,
        name: target.label,
        objectKey: target.objectKey,
        objectType: target.objectType,
        path: '',
      }),
    )
    return
  }

  await owner.switchObjectAtCurrentPosition?.({
    objectKey: target.objectKey,
  })
}
