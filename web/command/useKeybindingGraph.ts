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
  opts: Omit<KeybindingResolverOptions, 'overrideLayers'>,
): KeybindingGraph {
  const canonicalCommandIds = useMemo(
    () =>
      new Set(
        commands
          .map((state) => state.command?.commandId)
          .filter((id): id is string => Boolean(id)),
      ),
    [commands],
  )
  const localOverrides = useLocalKeybindingOverrides(
    opts.surface,
    canonicalCommandIds,
  )
  const accountOverrides = useAccountKeybindingOverrides(
    opts.surface,
    canonicalCommandIds,
  )
  const spaceOverrides = useSpaceKeybindingOverrides(
    opts.surface,
    canonicalCommandIds,
  )
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
        surface: opts.surface,
        platform,
        leaderCombo,
        overrideLayers,
      }),
    [commands, opts.surface, platform, leaderCombo, overrideLayers],
  )
}
