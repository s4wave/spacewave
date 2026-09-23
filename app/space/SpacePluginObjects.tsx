import { useState } from 'react'
import { useAbortSignal } from '@aptre/bldr-react'

import type { Space } from '@s4wave/sdk/space/space.js'
import { QuickstartRegistryResourceServiceClient } from '@s4wave/sdk/quickstart/registry/registry_srpc.pb.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import { Button } from '@s4wave/web/ui/button.js'

import { useQuickstartOptions } from '../quickstart/useQuickstartOptions.js'

/** SpacePluginObjects runs an installed plugin's creation action in the open Space. */
export function SpacePluginObjects({
  space,
  manifestId,
  onCreated,
}: {
  space: Space
  manifestId: string
  onCreated: (objectKey: string) => void
}) {
  // Discovery follows the installation; execution uses the mounted Space capability.
  const rootResource = RootContext.useContext()
  const root = rootResource.value
  const { spaceState } = SpaceContainerContext.useContext()
  const options = useQuickstartOptions(rootResource, spaceState.engineId)
  const signal = useAbortSignal([space, root])
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  // Cancellation ends observation; the accepted Quickstart operation stays atomic.
  async function create(quickstartId: string) {
    if (!root || pending || signal.aborted) return
    setPending(true)
    setError('')
    try {
      const result = await new QuickstartRegistryResourceServiceClient(
        root.client,
      ).ExecuteQuickstart({ quickstartId, spaceResourceId: space.id }, signal)
      if (!signal.aborted && result.indexPath) onCreated(result.indexPath)
    } catch (cause) {
      if (!signal.aborted) setError(String(cause))
    } finally {
      if (!signal.aborted) setPending(false)
    }
  }

  // Only this plugin's registered actions appear beside its build and preview.
  return (
    <div className="flex flex-wrap gap-2">
      {options.flatMap((option) =>
        option.dynamic && option.pluginId === manifestId
          ? [
              <Button
                key={option.id}
                size="sm"
                variant="ghost"
                disabled={!root || pending}
                onClick={() => void create(option.id)}
              >
                {pending ? 'Creating…' : `Create ${option.name}`}
              </Button>,
            ]
          : [],
      )}
      {error && (
        <p role="alert" className="text-destructive text-xs">
          {error}
        </p>
      )}
    </div>
  )
}
