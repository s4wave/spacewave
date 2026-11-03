import { LuShield } from '@s4wave/web/images/icons.js'

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

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export interface SecuritySectionProps {
  account?: Resource<Account>
  retainStepUp?: boolean
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

// SecuritySection wraps SessionLockSection and SecurityLevelSection into a
// single collapsible section.
export function SecuritySection({
  sx,
  account,
  retainStepUp = false,
  open,
  onOpenChange,
}: SecuritySectionProps & { sx?: stylex.StyleXStyles; className?: never }) {
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
      sx={sx}
      title="Security"
      icon={<LuShield sx={stylexStyles.rule1} />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
    >
      <div sx={false}>
        <SessionLockSection sx={nativeChildSpacing.s3} embedded />
        {!isLocal && accountResource.value && (
          <SecurityLevelSection
            sx={nativeChildSpacing.s3}
            account={accountResource}
            retainStepUp={retainStepUp}
            embedded
          />
        )}
      </div>
    </CollapsibleSection>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
