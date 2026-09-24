import { useCallback } from 'react'

import {
  useResourceValue,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Root } from '@s4wave/sdk/root'
import type {
  ObjectWizard,
  WatchWizardsResponse,
} from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { ObjectWizardRegistryResourceServiceClient } from '@s4wave/sdk/world/wizard/wizard_srpc.pb.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

// useWizardInstanceKey returns the plugin installation key of the current
// Space: its world engine id. Outside a Space only global wizards are visible.
function useWizardInstanceKey(): string {
  return SpaceContainerContext.useContextSafe()?.spaceState.engineId ?? ''
}

// useObjectWizards watches the object wizards visible to the current Space.
export function useObjectWizards(): Resource<WatchWizardsResponse> {
  const rootResource = RootContext.useContext()
  const instanceKey = useWizardInstanceKey()
  return useStreamingResource(
    rootResource,
    (root: Root, signal: AbortSignal) =>
      new ObjectWizardRegistryResourceServiceClient(root.client).WatchWizards(
        { instanceKey },
        signal,
      ),
    [instanceKey],
  )
}

// useListObjectWizards returns a function that reads the object wizards
// visible to the current Space, for callers that cannot wait for the watch.
export function useListObjectWizards(): (
  signal?: AbortSignal,
) => Promise<ObjectWizard[]> {
  const root = useResourceValue(RootContext.useContext())
  const instanceKey = useWizardInstanceKey()
  return useCallback(
    async (signal?: AbortSignal) => {
      if (!root) return []
      const response = await new ObjectWizardRegistryResourceServiceClient(
        root.client,
      ).ListWizards({ instanceKey }, signal)
      return response.wizards ?? []
    },
    [root, instanceKey],
  )
}
