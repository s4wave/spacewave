import { useState } from 'react'
import { LuCheck, LuCopy, LuDatabase, LuKey } from '@s4wave/web/images/icons.js'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'
import { formatBytes } from '@s4wave/web/transform/TransformConfigDisplay.js'
import type { SessionCryptoInfo } from '@s4wave/sdk/session/session.pb.js'
import { EntityKeypairsSection } from './EntityKeypairsSection.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-green-500'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

export interface CryptoKeysSectionProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

// CryptoKeysSection wraps crypto identity display and entity keypairs into a
// single collapsible section with a compact layout.
export function CryptoKeysSection({
  open,
  onOpenChange,
}: CryptoKeysSectionProps) {
  const ns = useStateNamespace(['session-settings'])
  const [storedOpen, setStoredOpen] = useStateAtom(ns, 'crypto', false)
  const sectionOpen = open ?? storedOpen
  const handleOpenChange = onOpenChange ?? setStoredOpen

  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { sessionInfo, providerId } = useSessionInfo(session)
  const isLocal = providerId === 'local'
  const crypto = sessionInfo?.cryptoInfo

  if (!crypto && !isLocal) return null

  return (
    <CollapsibleSection
      title="Crypto & Keys"
      icon={<LuKey sx={stylexStyles.rule1} />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
    >
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
        }
      >
        {crypto && <CryptoIdentity crypto={crypto} />}
        {isLocal && (
          <>
            {crypto && <div sx={stylexStyles2.rule1} />}
            <EntityKeypairsSection embedded />
          </>
        )}
      </div>
    </CollapsibleSection>
  )
}

// CryptoIdentity shows the session key type, a public key export, and the
// space and storage totals.
function CryptoIdentity({
  sx,
  crypto,
}: { crypto: SessionCryptoInfo } & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const [pemCopied, setPemCopied] = useState(false)
  const handleCopyPem = () => {
    const pem = crypto.publicKeyPem
    if (!pem) return
    void navigator.clipboard.writeText(pem)
    setPemCopied(true)
    setTimeout(() => setPemCopied(false), 2000)
  }

  return (
    <div sx={sx}>
      <div sx={[nativeChildSpacing.s2, stylexStyles2.rule2]}>
        {crypto.keyType && (
          <span sx={stylexStyles2.rule3}>{crypto.keyType}</span>
        )}
        {crypto.publicKeyPem && (
          <button
            type="button"
            onClick={handleCopyPem}
            sx={stylexStyles2.rule4}
            aria-label={pemCopied ? 'Copied!' : 'Copy public key PEM'}
          >
            {pemCopied ? (
              <LuCheck sx={stylexStyles.rule2} />
            ) : (
              <LuCopy sx={stylexStyles.rule3} />
            )}
            <span>{pemCopied ? 'Copied' : 'Export Public Key (PEM)'}</span>
          </button>
        )}
      </div>
      {(crypto.spaceCount ?? 0) > 0 && (
        <div sx={[nativeChildSpacing.s2, stylexStyles2.rule5]}>
          <div sx={stylexStyles2.rule6}>
            <LuDatabase sx={stylexStyles.rule3} />
            <span>
              {crypto.spaceCount} {crypto.spaceCount === 1 ? 'space' : 'spaces'}
            </span>
          </div>
          {(crypto.totalStorageBytes ?? 0n) > 0n && (
            <div sx={stylexStyles2.rule3}>
              {formatBytes(crypto.totalStorageBytes ?? 0n)}
            </div>
          )}
        </div>
      )}
    </div>
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
