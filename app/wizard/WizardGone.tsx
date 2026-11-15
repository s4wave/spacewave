import { useCallback } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { lookupWizardEnding } from '@s4wave/sdk/world/wizard/wizard-ending.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import { getObjectDisplayName } from '@s4wave/web/space/object-tree.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import type { LoadingView } from '@s4wave/web/ui/loading/types.js'
import { useShellTabsIfPresent, useTabId } from '../ShellTabContext.js'

// WizardEnding is how a removed wizard ended, as the World records it:
// checking while the ending is read, unreadable when the read failed,
// finished as the object its Finish opens, or deleted, by a Cancel or a plain
// delete, or because the object it finished as was deleted since.
export type WizardEnding =
  | { status: 'checking' }
  | { status: 'unreadable'; error: string; retry: () => void }
  | { status: 'finished'; targetKey: string }
  | { status: 'deleted' }

// wizardEnding is the ending a watch of lookupWizardEnding shows.
export function wizardEnding(
  resource: Resource<string | undefined>,
): WizardEnding {
  if (resource.error) {
    return {
      status: 'unreadable',
      error: resource.error.message,
      retry: resource.retry,
    }
  }
  if (resource.loading) return { status: 'checking' }
  if (resource.value) return { status: 'finished', targetKey: resource.value }
  return { status: 'deleted' }
}

// wizardEndingView is the card each ending shows. Every ending offers a next
// step besides closing the tab: the finished object, or the Space.
export function wizardEndingView(
  ending: WizardEnding,
  actions: { open: (key: string) => void; openSpace: () => void },
): LoadingView {
  switch (ending.status) {
    case 'checking':
      return { state: 'loading', title: 'Checking how this wizard ended' }
    case 'unreadable':
      return {
        state: 'error',
        title: 'Could not check how this wizard ended',
        error: ending.error,
        onRetry: ending.retry,
      }
    case 'finished': {
      const name = getObjectDisplayName(ending.targetKey)
      return {
        state: 'synced',
        title: 'Wizard finished',
        detail: `It finished with ${name}.`,
        onRetry: () => actions.open(ending.targetKey),
        retryLabel: `Open ${name}`,
      }
    }
    case 'deleted':
      return {
        state: 'error',
        title: 'Wizard deleted',
        detail: 'This wizard no longer exists.',
        onRetry: actions.openSpace,
        retryLabel: 'Open Space',
      }
  }
}

// WizardGone shows how the wizard at wizardKey ended once it is removed, such
// as by a Finish or Cancel in another tab, or by this tab's own Finish whose
// reply was lost before a reload. It watches the World's record of the
// ending, so a target deleted meanwhile turns finished into deleted. It
// offers no Retry: a Finish of a removed wizard is refused.
export function WizardGone({ wizardKey }: { wizardKey: string }) {
  const { spaceWorldResource, navigateToObjects, navigateToRoot } =
    SpaceContainerContext.useContext()
  const endingResource = useWorldQuery(
    spaceWorldResource,
    (world, signal) => lookupWizardEnding(world, wizardKey, signal),
    [wizardKey],
  )
  const ending = wizardEnding(endingResource)

  const tabId = useTabId()
  const closeShellTab = useShellTabsIfPresent()?.closeShellTab
  const closeTab = useCallback(() => {
    if (tabId) closeShellTab?.(tabId)
  }, [tabId, closeShellTab])
  const open = useCallback(
    (key: string) => navigateToObjects([key]),
    [navigateToObjects],
  )
  return (
    <div className="flex flex-1 items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <LoadingCard
          view={{
            ...wizardEndingView(ending, { open, openSpace: navigateToRoot }),
            onCancel: tabId && closeShellTab ? closeTab : undefined,
            cancelLabel: 'Close tab',
          }}
        />
      </div>
    </div>
  )
}
