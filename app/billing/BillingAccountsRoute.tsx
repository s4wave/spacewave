import { useCallback } from 'react'
import { LuCreditCard } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { BillingAccountsPage } from './BillingAccountsPage.js'

// BillingAccountsRoute renders the billing-account list within the session
// frame so the session bottom bar remains available on the billing list page.
// Billing accounts live in Spacewave Cloud, so a local session shows why it
// has none instead of the list.
export function BillingAccountsRoute() {
  const session = useResourceValue(SessionContext.useContext())
  const { providerId, isCloud } = useSessionInfo(session)

  let content = <BillingAccountsLoading />
  if (isCloud) {
    content = <BillingAccountsPage />
  } else if (providerId) {
    content = <LocalBillingAccounts />
  }
  return <SessionFrame>{content}</SessionFrame>
}

// BillingAccountsLoading waits for the session's provider.
function BillingAccountsLoading() {
  return (
    <div className="flex h-full w-full items-start justify-center pt-16">
      <div className="w-full max-w-sm px-4">
        <LoadingCard
          view={{
            state: 'active',
            title: 'Loading billing accounts',
            detail: 'Reading which provider this session uses.',
          }}
        />
      </div>
    </div>
  )
}

// LocalBillingAccounts explains that a local session has no billing accounts.
function LocalBillingAccounts() {
  const navigate = useNavigate()
  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  return (
    <div className="relative flex h-full w-full items-start justify-center overflow-y-auto pt-16 pb-8">
      <BackButton floating onClick={handleBack}>
        Back
      </BackButton>
      <div className="w-full max-w-md px-4">
        <EmptyState
          icon={<LuCreditCard className="text-foreground-alt size-7" />}
          title="No billing on a local session"
          description="This session keeps everything on this device. Billing accounts belong to a Spacewave Cloud account; sign in to one to create and manage them."
          variant="panel"
        />
      </div>
    </div>
  )
}
