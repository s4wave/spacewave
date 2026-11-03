import { useMemo, useSyncExternalStore } from 'react'
import { LuDownload } from '@s4wave/web/images/icons.js'

import { LegalPageLayout } from '@s4wave/app/landing/LegalPageLayout.js'
import { detectPlatform } from '@s4wave/web/platform/detect-platform.js'

import { CliSection } from './CliSection.js'
import {
  DOWNLOAD_MANIFEST,
  resolvePrimaryEntry,
  type DownloadOS,
} from './manifest.js'
import { PlatformSection } from './PlatformSection.js'
import { PrimaryDownloadButton } from './PrimaryDownloadButton.js'
import { WindowsBypassNotice } from './WindowsBypassNotice.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 14)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
  },
  rule2: {
    display: 'flex',
    width: '100%',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule3: {
    display: 'flex',
    width: '100%',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule4: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
  },
})

export const metadata = {
  title: 'Download Spacewave',
  description:
    'Download the Spacewave desktop app and CLI for macOS, Windows, and Linux. Auto-detected build for your platform plus every per-arch option.',
  canonicalPath: '/download',
}

export const cliMetadata = {
  ...metadata,
  title: 'Download Spacewave CLI',
  description:
    'Download the Spacewave CLI for macOS, Windows, and Linux. Auto-detected build for your platform plus every per-arch option.',
  canonicalPath: '/download/cli',
}

const PLATFORM_ORDER: readonly DownloadOS[] = ['macos', 'windows', 'linux']

// subscribeToBrowserPlatform is static because a document's platform does not change.
const subscribeToBrowserPlatform = () => () => {}
// readClientEnvironment becomes true after React completes the server snapshot render.
const readClientEnvironment = () => true
// readServerEnvironment keeps the first hydration render identical to prerendered HTML.
const readServerEnvironment = () => false

// DownloadPage renders the desktop and CLI download landing page. The
// installer and CLI sections share the platform manifest but render
// separately so /download/cli can render the CLI-focused view.
export function DownloadPage({ cliOnly = false }: { cliOnly?: boolean }) {
  // Use the neutral server view until hydration can safely select this browser's build.
  const isClient = useSyncExternalStore(
    subscribeToBrowserPlatform,
    readClientEnvironment,
    readServerEnvironment,
  )
  const detected = useMemo(
    () => (isClient ? detectPlatform(navigator) : null),
    [isClient],
  )

  // Select both primary downloads from the same hydrated platform snapshot.
  const installerPrimary = useMemo(
    () => resolvePrimaryEntry(detected, DOWNLOAD_MANIFEST, 'installer'),
    [detected],
  )
  const cliPrimary = useMemo(
    () => resolvePrimaryEntry(detected, DOWNLOAD_MANIFEST, 'cli'),
    [detected],
  )

  // Keep every architecture available alongside the detected primary choice.
  const installerGroups = useMemo(
    () =>
      PLATFORM_ORDER.map((os) => ({
        os,
        entries: DOWNLOAD_MANIFEST.filter(
          (e) => e.kind === 'installer' && e.os === os,
        ),
      })),
    [],
  )
  const cliGroups = useMemo(
    () =>
      PLATFORM_ORDER.map((os) => ({
        os,
        entries: DOWNLOAD_MANIFEST.filter(
          (e) => e.kind === 'cli' && e.os === os,
        ),
      })),
    [],
  )

  return (
    <LegalPageLayout
      icon={<LuDownload sx={stylexStyles.rule1} />}
      title={cliOnly ? 'Download Spacewave CLI' : 'Download Spacewave'}
      subtitle={
        cliOnly
          ? 'Standalone command-line builds for macOS, Windows, and Linux.'
          : 'Self-hosted in your browser. Available for macOS, Windows, and Linux.'
      }
    >
      <section sx={stylexStyles2.rule1}>
        {!cliOnly && (
          <section sx={stylexStyles2.rule2}>
            <PrimaryDownloadButton entry={installerPrimary} />

            <div sx={stylexStyles2.rule3}>
              {installerGroups.map(({ os, entries }) => (
                <div key={os} sx={stylexStyles2.rule4}>
                  <PlatformSection os={os} entries={entries} />
                  {os === 'windows' && entries.some((e) => e.unsigned) && (
                    <WindowsBypassNotice />
                  )}
                </div>
              ))}
            </div>
          </section>
        )}

        <CliSection primary={cliPrimary} groups={cliGroups} />
      </section>
    </LegalPageLayout>
  )
}
