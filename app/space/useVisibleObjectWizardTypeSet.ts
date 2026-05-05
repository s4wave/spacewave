import { useCallback, useMemo } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { SpaceContext } from '@s4wave/web/contexts/contexts.js'

import { normalizeObjectWizards } from './object-wizards.js'

// useVisibleObjectWizardTypeSet returns the set of creatable object type IDs
// visible for the current build mode.
export function useVisibleObjectWizardTypeSet(): Set<string> {
  const spaceResource = SpaceContext.useContext()
  const wizardState = useStreamingResource(
    spaceResource,
    useCallback((space, signal) => space.watchWizards(signal), []),
    [],
  )

  return useMemo(
    () =>
      new Set(
        normalizeObjectWizards(wizardState.value?.wizards ?? []).map(
          (wizard) => wizard.typeId ?? '',
        ),
      ),
    [wizardState.value?.wizards],
  )
}
