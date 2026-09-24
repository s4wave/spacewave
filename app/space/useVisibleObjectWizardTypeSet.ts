import { useMemo } from 'react'

import { useExperimentalCreatorsEnabled } from '../creator-visibility.js'
import { normalizeObjectWizards } from './object-wizards.js'
import { useObjectWizards } from './useObjectWizards.js'

// useVisibleObjectWizardTypeSet returns the set of creatable object type IDs
// visible for the current browser.
export function useVisibleObjectWizardTypeSet(): Set<string> {
  const experimentalCreatorsEnabled = useExperimentalCreatorsEnabled()
  const wizardState = useObjectWizards()

  return useMemo(
    () =>
      new Set(
        normalizeObjectWizards(
          wizardState.value?.wizards ?? [],
          experimentalCreatorsEnabled,
        ).map((wizard) => wizard.typeId ?? ''),
      ),
    [experimentalCreatorsEnabled, wizardState.value?.wizards],
  )
}
