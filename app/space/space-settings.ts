import {
  SetSpaceIndexPathOp,
  SetSpaceSettingsOp,
} from '@s4wave/core/space/world/ops/ops.pb.js'
import {
  SET_SPACE_INDEX_PATH_OP_ID,
  SET_SPACE_SETTINGS_OP_ID,
} from '@s4wave/core/space/world/ops/set-space-settings.js'
import { SPACE_SETTINGS_OBJECT_KEY } from '@s4wave/core/space/world/world.js'
import type { SpaceSettings } from '@s4wave/core/space/world/world.pb.js'
import { type KeybindingOverrideSet as ProtoKeybindingOverrideSet } from '@s4wave/sdk/command/command.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// applySpaceIndexPath updates the default object without dropping other settings.
export async function applySpaceIndexPath(
  spaceWorld: IWorldState,
  indexPath: string,
  options: {
    sender?: string
    signal?: AbortSignal
    expectedIndexPath?: string
  } = {},
): Promise<void> {
  const op: SetSpaceIndexPathOp = {
    indexPath,
    timestamp: new Date(),
    expectedIndexPath: options.expectedIndexPath,
  }
  const opData = SetSpaceIndexPathOp.toBinary(op)
  if (options.signal) {
    await spaceWorld.applyWorldOp(
      SET_SPACE_INDEX_PATH_OP_ID,
      opData,
      options.sender ?? '',
      options.signal,
    )
    return
  }

  await spaceWorld.applyWorldOp(
    SET_SPACE_INDEX_PATH_OP_ID,
    opData,
    options.sender ?? '',
  )
}

// applySpaceKeybindingOverrides updates Space shortcut overrides without dropping other settings.
export async function applySpaceKeybindingOverrides(
  spaceWorld: IWorldState,
  currentSettings: SpaceSettings | undefined,
  overrideSet: ProtoKeybindingOverrideSet,
  expectedOverrideSet: ProtoKeybindingOverrideSet,
  sender = '',
  abortSignal?: AbortSignal,
): Promise<void> {
  const settings: SpaceSettings = {
    ...currentSettings,
    indexPath: currentSettings?.indexPath ?? '',
    pluginIds: [...(currentSettings?.pluginIds ?? [])],
    keybindingOverrides: overrideSet,
  }
  const op: SetSpaceSettingsOp = {
    objectKey: SPACE_SETTINGS_OBJECT_KEY,
    settings,
    overwrite: true,
    timestamp: new Date(),
    expectedKeybindingOverrides: expectedOverrideSet,
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
