import { SetSpaceSettingsOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { SPACE_SETTINGS_OBJECT_KEY } from '@s4wave/core/space/world/world.js'
import type { SpaceSettings } from '@s4wave/core/space/world/world.pb.js'
import {
  keybindingOverrideSetToProto,
  type KeybindingOverrideSet,
} from '@s4wave/web/command/keybinding-overrides.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// applySpaceIndexPath updates the default object without dropping other settings.
export async function applySpaceIndexPath(
  spaceWorld: IWorldState,
  currentSettings: SpaceSettings | undefined,
  indexPath: string,
  sender = '',
  abortSignal?: AbortSignal,
): Promise<void> {
  const settings: SpaceSettings = {
    ...(currentSettings ?? {}),
    indexPath,
    pluginIds: [...(currentSettings?.pluginIds ?? [])],
  }
  const op: SetSpaceSettingsOp = {
    objectKey: SPACE_SETTINGS_OBJECT_KEY,
    settings,
    overwrite: true,
    timestamp: new Date(),
  }
  const opData = SetSpaceSettingsOp.toBinary(op)
  if (abortSignal) {
    await spaceWorld.applyWorldOp(
      SET_SPACE_SETTINGS_OP_ID,
      opData,
      sender,
      abortSignal,
    )
    return
  }

  await spaceWorld.applyWorldOp(SET_SPACE_SETTINGS_OP_ID, opData, sender)
}

// applySpaceKeybindingOverrides updates Space shortcut overrides without dropping other settings.
export async function applySpaceKeybindingOverrides(
  spaceWorld: IWorldState,
  currentSettings: SpaceSettings | undefined,
  overrideSet: KeybindingOverrideSet,
  sender = '',
  abortSignal?: AbortSignal,
): Promise<void> {
  const settings: SpaceSettings = {
    ...(currentSettings ?? {}),
    indexPath: currentSettings?.indexPath ?? '',
    pluginIds: [...(currentSettings?.pluginIds ?? [])],
    keybindingOverrides: keybindingOverrideSetToProto(overrideSet),
  }
  const op: SetSpaceSettingsOp = {
    objectKey: SPACE_SETTINGS_OBJECT_KEY,
    settings,
    overwrite: true,
    timestamp: new Date(),
  }
  const opData = SetSpaceSettingsOp.toBinary(op)
  if (abortSignal) {
    await spaceWorld.applyWorldOp(
      SET_SPACE_SETTINGS_OP_ID,
      opData,
      sender,
      abortSignal,
    )
    return
  }

  await spaceWorld.applyWorldOp(SET_SPACE_SETTINGS_OP_ID, opData, sender)
}
