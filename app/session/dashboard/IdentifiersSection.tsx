import { LuFingerprint } from '@s4wave/web/images/icons.js'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export interface IdentifiersSectionProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

// IdentifiersSection wraps session, peer, and account IDs in a collapsible
// section. Defaults to closed since these are rarely needed.
export function IdentifiersSection({
  sx,
  open,
  onOpenChange,
}: IdentifiersSectionProps & { sx?: stylex.StyleXStyles; className?: never }) {
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
      sx={sx}
      title="Identifiers"
      icon={<LuFingerprint sx={stylexStyles.rule1} />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
    >
      <div>
        <CopyableField
          sx={nativeChildSpacing.s2}
          label="Session ID"
          value={sessionId}
        />
        <CopyableField
          sx={nativeChildSpacing.s2}
          label="Peer ID"
          value={peerId}
        />
        <CopyableField
          sx={nativeChildSpacing.s2}
          label="Account ID"
          value={accountId}
        />
      </div>
    </CollapsibleSection>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
