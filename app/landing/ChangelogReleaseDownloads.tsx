import { useCallback, useId, useMemo, useState } from 'react'
import { LuMonitorDown } from 'react-icons/lu'

import { detectPlatform } from '@s4wave/web/platform/detect-platform.js'
import {
  resolveInstallerEntries,
  resolvePrimaryEntry,
  type DownloadOS,
} from '@s4wave/app/download/manifest.js'
import { PlatformSection } from '@s4wave/app/download/PlatformSection.js'
import { PrimaryDownloadButton } from '@s4wave/app/download/PrimaryDownloadButton.js'

const PLATFORM_ORDER: readonly DownloadOS[] = ['macos', 'windows', 'linux']

// ChangelogReleaseDownloads renders the desktop-app download options for a
// changelog release page. It defaults to the latest release's artifacts and
// highlights the detected platform; the "exact version" checkbox repoints
// every link at the release whose notes are being read.
export function ChangelogReleaseDownloads({ version }: { version: string }) {
  const [exactVersion, setExactVersion] = useState(false)
  const checkboxId = useId()

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
        entries: entries.filter((e) => e.os === os),
      })),
    [entries],
  )

  const toggleExact = useCallback(() => setExactVersion((prev) => !prev), [])

  return (
    <section className="flex w-full flex-col gap-8">
      <header className="flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <LuMonitorDown className="text-foreground-alt size-5" />
          <h2 className="text-foreground text-2xl font-semibold @lg:text-3xl">
            Download the desktop app
          </h2>
        </div>
        <p className="text-foreground-alt text-sm @lg:text-base">
          The buttons install the latest Spacewave release. Tick the box below
          to grab the exact build that shipped with v{version}.
        </p>
      </header>

      <PrimaryDownloadButton entry={primary} />

      <label
        htmlFor={checkboxId}
        className="text-foreground-alt hover:text-foreground flex cursor-pointer items-center gap-2 text-sm select-none"
      >
        <input
          id={checkboxId}
          type="checkbox"
          checked={exactVersion}
          onChange={toggleExact}
          className="accent-brand size-4 cursor-pointer"
        />
        Download this exact version (v{version})
      </label>

      <div className="flex w-full flex-col gap-10">
        {groups.map(({ os, entries }) => (
          <PlatformSection key={os} os={os} entries={entries} />
        ))}
      </div>
    </section>
  )
}
