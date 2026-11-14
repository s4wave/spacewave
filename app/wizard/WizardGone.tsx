import { useCallback } from 'react'

import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useShellTabsIfPresent, useTabId } from '../ShellTabContext.js'

// WizardGone tells the person the wizard no longer exists, such as after a
// Finish or Cancel in another tab, and offers to close the shell tab that
// showed it. It offers no Retry: a Finish of a removed wizard is refused.
export function WizardGone() {
  const tabId = useTabId()
  const closeShellTab = useShellTabsIfPresent()?.closeShellTab
  const closeTab = useCallback(() => {
    if (tabId) closeShellTab?.(tabId)
  }, [tabId, closeShellTab])
  return (
    <div className="flex flex-1 items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <LoadingCard
          view={{
            state: 'error',
            title: 'Wizard deleted',
            detail: 'This wizard no longer exists.',
            onCancel: tabId && closeShellTab ? closeTab : undefined,
            cancelLabel: 'Close tab',
          }}
        />
      </div>
    </div>
  )
}
