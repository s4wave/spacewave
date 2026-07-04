import { useMemo } from 'react'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import {
  resolveKeybindings,
  type KeybindingGraph,
  type KeybindingResolverOptions,
} from './KeybindingResolver.js'
import type { KeybindingOverrideLayer } from './keybinding-overrides.js'
import { useLocalKeybindingOverrides } from './useLocalKeybindingOverrides.js'
import { useAccountKeybindingOverrides } from './useAccountKeybindingOverrides.js'
import { useSpaceKeybindingOverrides } from './useSpaceKeybindingOverrides.js'

export function useKeybindingGraph(
  commands: CommandState[],
  opts: Omit<KeybindingResolverOptions, 'overrideLayers'> = {},
): KeybindingGraph {
  const localOverrides = useLocalKeybindingOverrides()
  const accountOverrides = useAccountKeybindingOverrides()
  const spaceOverrides = useSpaceKeybindingOverrides()
  const platform = opts.platform
  const leaderCombo = opts.leaderCombo
  const overrideLayers = useMemo(
    () =>
      [
        localOverrides.layer,
        accountOverrides.layer,
        spaceOverrides.layer,
      ].filter((layer): layer is KeybindingOverrideLayer => Boolean(layer)),
    [localOverrides.layer, accountOverrides.layer, spaceOverrides.layer],
  )
  return useMemo(
    () =>
      resolveKeybindings(commands, {
        platform,
        leaderCombo,
        overrideLayers,
      }),
    [commands, platform, leaderCombo, overrideLayers],
  )
}
