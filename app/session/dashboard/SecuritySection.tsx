import { LuShield } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { SessionLockSection } from './SessionLockSection.js'
import { SecurityLevelSection } from './SecurityLevelSection.js'

export interface SecuritySectionProps {
  account?: Resource<Account>
  retainStepUp?: boolean
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

// SecuritySection wraps SessionLockSection and SecurityLevelSection into a
// single collapsible section.
export function SecuritySection({
  account,
  retainStepUp = false,
  open,
  onOpenChange,
}: SecuritySectionProps) {
  const ns = useStateNamespace(['session-settings'])
  const [storedOpen, setStoredOpen] = useStateAtom(ns, 'security', true)
  const sectionOpen = open ?? storedOpen
  const handleOpenChange = onOpenChange ?? setStoredOpen

  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { providerId, accountId } = useSessionInfo(session)
  const isLocal = providerId === 'local'
  const mountedAccount = useMountAccount(providerId, accountId, account == null)
  const accountResource = account ?? mountedAccount

  return (
    <CollapsibleSection
      title="Security"
      icon={<LuShield className="h-3.5 w-3.5" />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
    >
      <div className="space-y-3">
        <SessionLockSection embedded />
        {!isLocal && accountResource.value && (
          <SecurityLevelSection
            account={accountResource}
            retainStepUp={retainStepUp}
            embedded
          />
        )}
      </div>
    </CollapsibleSection>
  )
}
