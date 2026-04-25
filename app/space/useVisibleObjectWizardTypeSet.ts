import { useCallback, useMemo } from 'react'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { SpaceContext } from '@s4wave/web/contexts/contexts.js'

import { normalizeObjectWizards } from './object-wizards.js'

// useVisibleObjectWizardTypeSet returns the set of creatable object type IDs
// visible for the current build mode.
export function useVisibleObjectWizardTypeSet(): Set<string> {
  const spaceResource = SpaceContext.useContext()
  const space = useResourceValue(spaceResource)
  const { data: wizards } = usePromise(
    useCallback((signal) => space?.listWizards(signal), [space]),
  )

  return useMemo(
    () =>
      new Set(
        normalizeObjectWizards(wizards ?? []).map(
          (wizard) => wizard.typeId ?? '',
        ),
      ),
    [wizards],
  )
}
