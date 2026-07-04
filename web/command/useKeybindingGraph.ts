import { useMemo } from 'react'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import {
  resolveKeybindings,
  type KeybindingGraph,
  type KeybindingResolverOptions,
} from './KeybindingResolver.js'
import { useLocalKeybindingOverrides } from './useLocalKeybindingOverrides.js'

export function useKeybindingGraph(
  commands: CommandState[],
  opts: Omit<KeybindingResolverOptions, 'overrideLayers'> = {},
): KeybindingGraph {
  const localOverrides = useLocalKeybindingOverrides()
  const platform = opts.platform
  const leaderCombo = opts.leaderCombo
  return useMemo(
    () =>
      resolveKeybindings(commands, {
        platform,
        leaderCombo,
        overrideLayers: [localOverrides.layer],
      }),
    [commands, platform, leaderCombo, localOverrides.layer],
  )
}
