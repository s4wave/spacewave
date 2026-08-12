import { useCallback, useId, useMemo, useState } from 'react'
import { LuMonitorDown } from 'react-icons/lu'

import {
  isReleaseVersion,
  resolveInstallerEntries,
  resolvePrimaryEntry,
  type DownloadOS,
} from '@s4wave/app/download/manifest.js'
import { PlatformSection } from '@s4wave/app/download/PlatformSection.js'
import { PrimaryDownloadButton } from '@s4wave/app/download/PrimaryDownloadButton.js'
import { cn } from '@s4wave/web/style/utils.js'
import { detectPlatform } from '@s4wave/web/platform/detect-platform.js'

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
    <section className="flex w-full flex-col gap-8">
      <header className="flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <LuMonitorDown className="text-foreground-alt size-5" />
          <h2 className="text-foreground text-2xl font-semibold @lg:text-3xl">
            Download the desktop app
          </h2>
        </div>
        <p className="text-foreground-alt text-sm @lg:text-base">
          Choose the newest Spacewave app or the build documented on this page.
        </p>
      </header>

      <fieldset className="flex flex-col gap-3">
        <legend className="text-foreground text-sm font-semibold">
          Choose a release
        </legend>
        <div className="grid gap-3 @md:grid-cols-2">
          <label
            className={cn(
              'focus-within:ring-brand/70 flex cursor-pointer gap-3 rounded-lg border p-4 focus-within:ring-2',
              releaseChoice === 'latest'
                ? 'border-brand/50 bg-brand/10'
                : 'border-foreground/10 bg-background-card/30 hover:border-foreground/20',
            )}
          >
            <input
              type="radio"
              name={choiceName}
              value="latest"
              checked={releaseChoice === 'latest'}
              onChange={selectLatest}
              className="accent-brand mt-1 size-4 shrink-0 cursor-pointer"
            />
            <span className="flex flex-col gap-1">
              <span className="text-foreground text-sm font-semibold">
                Latest release
              </span>
              <span className="text-foreground-alt text-xs leading-relaxed">
                Recommended. Downloads the newest published build.
              </span>
            </span>
          </label>
          <label
            className={cn(
              'focus-within:ring-brand/70 flex gap-3 rounded-lg border p-4 focus-within:ring-2',
              hasReleaseVersion
                ? 'cursor-pointer'
                : 'cursor-not-allowed opacity-50',
              releaseChoice === 'this-release'
                ? 'border-brand/50 bg-brand/10'
                : 'border-foreground/10 bg-background-card/30 hover:border-foreground/20',
            )}
          >
            <input
              type="radio"
              name={choiceName}
              value="this-release"
              checked={releaseChoice === 'this-release'}
              onChange={selectThisRelease}
              disabled={!hasReleaseVersion}
              className="accent-brand mt-1 size-4 shrink-0 cursor-pointer disabled:cursor-not-allowed"
            />
            <span className="flex flex-col gap-1">
              <span className="text-foreground text-sm font-semibold">
                This release{hasReleaseVersion ? ` · v${version}` : ''}
              </span>
              <span className="text-foreground-alt text-xs leading-relaxed">
                Downloads the exact build described in these release notes.
              </span>
            </span>
          </label>
        </div>
      </fieldset>

      <div className="flex flex-col gap-2">
        <p className="text-foreground-alt text-xs font-medium tracking-wider uppercase">
          Selected: {releaseLabel}
        </p>
        <PrimaryDownloadButton entry={primary} releaseLabel={releaseLabel} />
        {!primary && (
          <p className="text-foreground-alt text-center text-sm">
            We could not detect a supported build for this device. Choose your
            platform and architecture below.
          </p>
        )}
      </div>

      <div
        className="flex w-full flex-col gap-10"
        aria-label={`${releaseLabel} downloads`}
      >
        {groups.map(({ os, entries }) => (
          <PlatformSection key={os} os={os} entries={entries} />
        ))}
      </div>
    </section>
  )
}
