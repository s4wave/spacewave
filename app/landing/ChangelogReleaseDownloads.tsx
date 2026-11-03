import { useCallback, useId, useMemo, useState } from 'react'
import { LuMonitorDown } from '@s4wave/web/images/icons.js'

import {
  isReleaseVersion,
  resolveInstallerEntries,
  resolvePrimaryEntry,
  type DownloadOS,
} from '@s4wave/app/download/manifest.js'
import { PlatformSection } from '@s4wave/app/download/PlatformSection.js'
import { PrimaryDownloadButton } from '@s4wave/app/download/PrimaryDownloadButton.js'

import { detectPlatform } from '@s4wave/web/platform/detect-platform.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'flex',
    flexDirection: 'column',
    width: '100%',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: {
      default: 'var(--text-2xl)',
      '@container (min-width: 32rem)': 'var(--text-3xl)',
    },
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: {
      default: 'var(--tw-leading, var(--text-2xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-3xl--line-height))',
    },
  },
  rule5: {
    color: themeColors['--color-foreground-alt'],
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 28rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule9: {
    '--tw-ring-color': {
      default: null,
      ':focus-within': `color-mix(in oklab, ${themeColors['--color-brand']} 70%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-within':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    boxShadow: {
      default: null,
      ':focus-within':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    cursor: 'pointer',
    display: 'flex',
  },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule11: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule12: {
    accentColor: themeColors['--color-brand'],
    cursor: 'pointer',
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginTop: themeLayout['--spacing'],
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule13: {
    gap: themeLayout['--spacing'],
    display: 'flex',
    flexDirection: 'column',
  },
  rule14: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule15: {
    '--tw-ring-color': {
      default: null,
      ':focus-within': `color-mix(in oklab, ${themeColors['--color-brand']} 70%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-within':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    boxShadow: {
      default: null,
      ':focus-within':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    display: 'flex',
  },
  rule16: { cursor: 'pointer' },
  rule17: { cursor: 'not-allowed', opacity: 0.5 },
  rule18: {
    accentColor: themeColors['--color-brand'],
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginTop: themeLayout['--spacing'],
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule19: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
  },
  rule20: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
  },
  rule21: {
    gap: `calc(${themeLayout['--spacing']} * 10)`,
    display: 'flex',
    flexDirection: 'column',
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const PLATFORM_ORDER: readonly DownloadOS[] = ['macos', 'windows', 'linux']
type ReleaseChoice = 'latest' | 'this-release'

// ChangelogReleaseDownloads renders the desktop-app download options for a
// changelog release page. The selected release is the single source for every
// primary and manual download URL in the section.
export function ChangelogReleaseDownloads({ version }: { version: string }) {
  const [releaseChoice, setReleaseChoice] = useState<ReleaseChoice>('latest')
  const choiceName = useId()
  const hasReleaseVersion = isReleaseVersion(version)
  const exactVersion = releaseChoice === 'this-release' && hasReleaseVersion

  const detected = useMemo(() => {
    if (typeof navigator === 'undefined') return null
    return detectPlatform(navigator)
  }, [])

  const entries = useMemo(
    () => resolveInstallerEntries(exactVersion ? version : null),
    [exactVersion, version],
  )
  const primary = useMemo(
    () => resolvePrimaryEntry(detected, entries, 'installer'),
    [detected, entries],
  )
  const groups = useMemo(
    () =>
      PLATFORM_ORDER.map((os) => ({
        os,
        entries: entries.filter((entry) => entry.os === os),
      })),
    [entries],
  )

  const selectLatest = useCallback(() => setReleaseChoice('latest'), [])
  const selectThisRelease = useCallback(
    () => setReleaseChoice('this-release'),
    [],
  )
  const releaseLabel = exactVersion ? `v${version}` : 'Latest release'

  return (
    <section sx={stylexStyles2.rule1}>
      <header sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <LuMonitorDown sx={stylexStyles.rule1} />
          <h2 sx={stylexStyles2.rule4}>Download the desktop app</h2>
        </div>
        <p sx={stylexStyles2.rule5}>
          Choose the newest Spacewave app or the build documented on this page.
        </p>
      </header>

      <fieldset sx={stylexStyles2.rule6}>
        <legend sx={stylexStyles2.rule7}>Choose a release</legend>
        <div sx={stylexStyles2.rule8}>
          <label
            sx={[
              stylexStyles2.rule9,
              releaseChoice === 'latest'
                ? stylexStyles2.rule10
                : stylexStyles2.rule11,
            ]}
          >
            <input
              type="radio"
              name={choiceName}
              value="latest"
              checked={releaseChoice === 'latest'}
              onChange={selectLatest}
              sx={stylexStyles2.rule12}
            />
            <span sx={stylexStyles2.rule13}>
              <span sx={stylexStyles2.rule7}>Latest release</span>
              <span sx={stylexStyles2.rule14}>
                Recommended. Downloads the newest published build.
              </span>
            </span>
          </label>
          <label
            sx={[
              stylexStyles2.rule15,
              hasReleaseVersion ? stylexStyles2.rule16 : stylexStyles2.rule17,
              releaseChoice === 'this-release'
                ? stylexStyles2.rule10
                : stylexStyles2.rule11,
            ]}
          >
            <input
              type="radio"
              name={choiceName}
              value="this-release"
              checked={releaseChoice === 'this-release'}
              onChange={selectThisRelease}
              disabled={!hasReleaseVersion}
              sx={stylexStyles2.rule18}
            />
            <span sx={stylexStyles2.rule13}>
              <span sx={stylexStyles2.rule7}>
                This release{hasReleaseVersion ? ` · v${version}` : ''}
              </span>
              <span sx={stylexStyles2.rule14}>
                Downloads the exact build described in these release notes.
              </span>
            </span>
          </label>
        </div>
      </fieldset>

      <div sx={stylexStyles2.rule2}>
        <p sx={stylexStyles2.rule19}>Selected: {releaseLabel}</p>
        <PrimaryDownloadButton entry={primary} releaseLabel={releaseLabel} />
        {!primary && (
          <p sx={stylexStyles2.rule20}>
            We could not detect a supported build for this device. Choose your
            platform and architecture below.
          </p>
        )}
      </div>

      <div sx={stylexStyles2.rule21} aria-label={`${releaseLabel} downloads`}>
        {groups.map(({ os, entries }) => (
          <PlatformSection key={os} os={os} entries={entries} />
        ))}
      </div>
    </section>
  )
}
