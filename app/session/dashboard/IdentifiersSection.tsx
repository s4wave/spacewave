import { LuFingerprint } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'

export interface IdentifiersSectionProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

// IdentifiersSection wraps session, peer, and account IDs in a collapsible
// section. Defaults to closed since these are rarely needed.
export function IdentifiersSection({
  open,
  onOpenChange,
}: IdentifiersSectionProps) {
  const ns = useStateNamespace(['session-settings'])
  const [storedOpen, setStoredOpen] = useStateAtom(ns, 'identifiers', false)
  const sectionOpen = open ?? storedOpen
  const handleOpenChange = onOpenChange ?? setStoredOpen

  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { sessionInfo } = useSessionInfo(session)

  const peerId = sessionInfo?.peerId ?? 'Unknown'
  const sessionId =
    sessionInfo?.sessionRef?.providerResourceRef?.id ?? 'Unknown'
  const accountId =
    sessionInfo?.sessionRef?.providerResourceRef?.providerAccountId ?? 'Unknown'

  return (
    <CollapsibleSection
      title="Identifiers"
      icon={<LuFingerprint className="h-3.5 w-3.5" />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
    >
      <div className="space-y-2">
        <CopyableField label="Session ID" value={sessionId} />
        <CopyableField label="Peer ID" value={peerId} />
        <CopyableField label="Account ID" value={accountId} />
      </div>
    </CollapsibleSection>
  )
}
