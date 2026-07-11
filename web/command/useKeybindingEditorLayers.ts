import { useMemo } from 'react'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import { resolveKeybindings } from './KeybindingResolver.js'
import type { KeybindingOverrideLayer } from './keybinding-overrides.js'
import { useAccountKeybindingOverrides } from './useAccountKeybindingOverrides.js'
import { useLocalKeybindingOverrides } from './useLocalKeybindingOverrides.js'
import { useSpaceKeybindingOverrides } from './useSpaceKeybindingOverrides.js'
import type {
  KeybindingEditorScope,
  KeybindingLayerController,
} from './component.js'

// useKeybindingEditorLayers owns the editable override layers and effective graph.
export function useKeybindingEditorLayers(commands: CommandState[]) {
  const local = useLocalKeybindingOverrides()
  const account = useAccountKeybindingOverrides()
  const space = useSpaceKeybindingOverrides()
  const controllers: Record<KeybindingEditorScope, KeybindingLayerController> =
    {
      local: {
        scope: 'local',
        label: 'Local',
        overrideSet: local.overrideSet,
        layer: local.layer,
        available: true,
        readOnly: false,
        loading: false,
        error: null,
        settingsEditable: true,
        setSettings: local.setSettings,
        setOverrideSet: local.setOverrideSet,
        setCommandBindings: local.setCommandBindings,
        addCommandBinding: local.addCommandBinding,
        clearCommandBindings: local.clearCommandBindings,
        clearCommandBindingId: local.clearCommandBindingId,
        removeCommandBinding: local.removeLocalCommandBinding,
        resetCommand: local.resetCommand,
        resetLayer: local.resetLayer,
      },
      account: {
        scope: 'account',
        label: 'Account',
        overrideSet: account.overrideSet,
        layer: account.layer,
        available: account.available,
        readOnly: account.readOnly,
        loading: account.loading,
        error: account.error,
        settingsEditable: true,
        setSettings: account.setSettings,
        setOverrideSet: account.setOverrideSet,
        setCommandBindings: account.setCommandBindings,
        addCommandBinding: account.addCommandBinding,
        clearCommandBindings: account.clearCommandBindings,
        clearCommandBindingId: account.clearCommandBindingId,
        removeCommandBinding: account.removeCommandBinding,
        resetCommand: account.resetCommand,
        resetLayer: account.resetLayer,
      },
      space: {
        scope: 'space',
        label: 'Space',
        overrideSet: space.overrideSet,
        layer: space.layer,
        available: space.available,
        readOnly: space.readOnly,
        loading: space.loading,
        error: space.error,
        settingsEditable: true,
        setSettings: space.setSettings,
        setOverrideSet: space.setOverrideSet,
        setCommandBindings: space.setCommandBindings,
        addCommandBinding: space.addCommandBinding,
        clearCommandBindings: space.clearCommandBindings,
        clearCommandBindingId: space.clearCommandBindingId,
        removeCommandBinding: space.removeCommandBinding,
        resetCommand: space.resetCommand,
        resetLayer: space.resetLayer,
      },
    }
  const overrideLayers = useMemo(
    () =>
      [local.layer, account.layer, space.layer].filter(
        (layer): layer is KeybindingOverrideLayer => Boolean(layer),
      ),
    [local.layer, account.layer, space.layer],
  )
  const bindingGraph = useMemo(
    () => resolveKeybindings(commands, { overrideLayers }),
    [commands, overrideLayers],
  )

  return {
    accountOverridesAvailable: account.available,
    spaceOverridesAvailable: space.available,
    controllers,
    overrideLayers,
    bindingGraph,
  }
}
