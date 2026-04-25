import { useMemo } from 'react'
import { LuDownload } from 'react-icons/lu'

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

export const metadata = {
  title: 'Download Spacewave',
  description:
    'Download the Spacewave desktop app and CLI for macOS, Windows, and Linux. Auto-detected build for your platform plus every per-arch option.',
  canonicalPath: '/download',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

const PLATFORM_ORDER: readonly DownloadOS[] = ['macos', 'windows', 'linux']

// DownloadPage renders the desktop and CLI download landing page. The
// installer and CLI sections share the platform manifest but render
// separately so the CLI section heading can carry a stable #cli anchor
// for /download#cli deep links.
export function DownloadPage() {
  const detected = useMemo(() => {
    if (typeof navigator === 'undefined') return null
    return detectPlatform(navigator)
  }, [])

  const installerPrimary = useMemo(
    () => resolvePrimaryEntry(detected, DOWNLOAD_MANIFEST, 'installer'),
    [detected],
  )
  const cliPrimary = useMemo(
    () => resolvePrimaryEntry(detected, DOWNLOAD_MANIFEST, 'cli'),
    [detected],
  )

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
      icon={<LuDownload className="h-6 w-6" />}
      title="Download Spacewave"
      subtitle="Self-hosted in your browser. Available for macOS, Windows, and Linux."
    >
      <section className="relative z-10 mx-auto flex w-full max-w-4xl flex-col items-center gap-14 px-4 pb-14 @lg:px-8 @lg:pb-16">
        <section className="flex w-full flex-col items-center gap-10">
          <PrimaryDownloadButton entry={installerPrimary} />

          <div className="flex w-full flex-col gap-10">
            {installerGroups.map(({ os, entries }) => (
              <div key={os} className="flex flex-col gap-4">
                <PlatformSection os={os} entries={entries} />
                {os === 'windows' && entries.some((e) => e.unsigned) && (
                  <WindowsBypassNotice />
                )}
              </div>
            ))}
          </div>
        </section>

        <CliSection primary={cliPrimary} groups={cliGroups} />
      </section>
    </LegalPageLayout>
  )
}
