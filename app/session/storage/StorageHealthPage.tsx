import { useCallback } from 'react'
import { LuDatabase, LuShieldCheck } from '@s4wave/web/images/icons.js'

import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'

import { StorageBackendsSection } from './StorageBackendsSection.js'
import { StorageHealth } from './StorageHealth.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    width: '100%',
    alignItems: 'flex-start',
    justifyContent: 'center',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-primary'],
    paddingTop: `calc(${themeLayout['--spacing']} * 16)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule2: {
    width: '100%',
    maxWidth: 'var(--container-md)',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule6: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flexShrink: 0,
    color: themeColors['--color-foreground'],
  },
})

interface StorageHealthPageProps {
  recovery?: boolean
  storageIncident?: boolean
}

// StorageHealthPage renders the session storage settings and dedicated recovery route.
export function StorageHealthPage({
  recovery = false,
  storageIncident = false,
}: StorageHealthPageProps) {
  const navigateSession = useSessionNavigate()
  const handleBack = useCallback(() => {
    navigateSession({ path: '' })
  }, [navigateSession])
  const handleOpenRecovery = useCallback(() => {
    navigateSession({ path: 'settings/storage/recovery' })
  }, [navigateSession])
  const handleLinkDevice = useCallback(() => {
    navigateSession({ path: 'setup/link-device' })
  }, [navigateSession])
  const handleUseCloud = useCallback(() => {
    navigateSession({ path: 'plan' })
  }, [navigateSession])

  const Icon = recovery ? LuShieldCheck : LuDatabase
  return (
    <div sx={stylexStyles2.rule1}>
      <main sx={stylexStyles2.rule2}>
        <BackButton floating onClick={handleBack}>
          Back
        </BackButton>
        <div sx={stylexStyles2.rule3}>
          <Icon sx={stylexStyles.rule1} aria-hidden="true" />
          <div>
            <h1 sx={stylexStyles2.rule4}>
              {recovery ? 'Storage recovery' : 'Storage health'}
            </h1>
            <p sx={stylexStyles2.rule5}>
              {recovery
                ? 'Review browser protection and backup options.'
                : 'See what this browser, this device, and sync activity report.'}
            </p>
          </div>
        </div>

        <StorageHealth
          mode={recovery ? 'recovery' : 'settings'}
          storageIncident={storageIncident}
          onOpenRecovery={recovery ? undefined : handleOpenRecovery}
          onLinkDevice={handleLinkDevice}
          onUseCloud={handleUseCloud}
        />
        {!recovery && (
          <div sx={stylexStyles2.rule6}>
            <StorageBackendsSection />
          </div>
        )}
      </main>
    </div>
  )
}
