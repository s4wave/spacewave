import { LuFolderOpen, LuPanelTopOpen, LuReplace } from 'react-icons/lu'

import type { BottomBarContextMenuItem } from '@s4wave/web/frame/bottom-bar-context.js'
import type { SpaceObjectActionTarget } from './object-tree.js'

export interface SpaceObjectNavigationActionHandlers {
  openDetails?: () => void
  openObject?: (target: SpaceObjectActionTarget) => void
  switchObjectHere?: (target: SpaceObjectActionTarget) => void | Promise<void>
}

export interface SpaceObjectNavigationActionOptions extends SpaceObjectNavigationActionHandlers {
  targets: readonly SpaceObjectActionTarget[]
  currentObjectKey?: string
}

export function createSpaceObjectNavigationActions({
  targets,
  currentObjectKey,
  openDetails,
  openObject,
  switchObjectHere,
}: SpaceObjectNavigationActionOptions): BottomBarContextMenuItem[] {
  const items: BottomBarContextMenuItem[] = []

  if (openDetails) {
    items.push({
      type: 'action',
      id: 'open-details',
      label: 'Open Details',
      icon: LuPanelTopOpen,
      onSelect: ({ openPrimaryOverlay }) => {
        openPrimaryOverlay()
        openDetails()
      },
    })
  }

  if (openObject) {
    items.push({
      type: 'group',
      id: 'browse-objects',
      label: 'Browse Objects',
      items: targetActions(targets, currentObjectKey, 'open', openObject),
    })
  }

  if (switchObjectHere) {
    items.push({
      type: 'group',
      id: 'switch-object-here',
      label: 'Switch Object Here',
      items: targetActions(
        targets,
        currentObjectKey,
        'switch',
        switchObjectHere,
      ),
    })
  }

  return items
}

function targetActions(
  targets: readonly SpaceObjectActionTarget[],
  currentObjectKey: string | undefined,
  actionIdPrefix: string,
  onSelect: (target: SpaceObjectActionTarget) => void | Promise<void>,
): BottomBarContextMenuItem[] {
  if (targets.length === 0) {
    return [
      {
        type: 'action',
        id: `${actionIdPrefix}-empty`,
        label: 'No visible objects',
        disabled: true,
        onSelect: () => {},
      },
    ]
  }

  return targets.map((target) => {
    const isCurrent = target.objectKey === currentObjectKey
    return {
      type: 'action',
      id: `${actionIdPrefix}:${target.objectKey}`,
      label: target.label,
      icon: actionIdPrefix === 'switch' ? LuReplace : LuFolderOpen,
      disabled: isCurrent,
      onSelect: () => onSelect(target),
    }
  })
}
