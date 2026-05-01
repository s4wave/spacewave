import { LuTerminal } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import { ArchTile } from './ArchTile.js'
import { CliInstallSnippet } from './CliInstallSnippet.js'
import type { DownloadEntry, DownloadOS } from './manifest.js'
import { PrimaryDownloadButton } from './PrimaryDownloadButton.js'

interface CliSectionProps {
  primary: DownloadEntry | null
  groups: ReadonlyArray<{ os: DownloadOS; entries: readonly DownloadEntry[] }>
}

const OS_HEADINGS: Record<DownloadOS, string> = {
  macos: 'macOS',
  windows: 'Windows',
  linux: 'Linux',
}

// CliSection renders the standalone CLI block on the download pages.
// Layout: heading and intro, platform-aware primary download button,
// then per-OS sub-sections each containing the install snippet and the
// per-arch tile grid.
export function CliSection({ primary, groups }: CliSectionProps) {
  return (
    <section
      id="cli"
      className="flex w-full scroll-mt-24 flex-col gap-8"
      aria-labelledby="cli-heading"
    >
      <header className="flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <LuTerminal className="text-foreground-alt h-5 w-5" />
          <h2
            id="cli-heading"
            className="text-foreground text-2xl font-bold select-none @lg:text-3xl"
          >
            Spacewave CLI
          </h2>
        </div>
        <p className="text-foreground-alt text-sm @lg:text-base">
          Standalone command-line binary. Install it alongside the desktop app
          to script your workflow, drive headless servers, or pipe spacewave
          output through other Unix tools.
        </p>
      </header>

      <PrimaryDownloadButton entry={primary} />

      <div className="flex flex-col gap-8">
        {groups.map(({ os, entries }) => (
          <CliPlatformGroup key={os} os={os} entries={entries} />
        ))}
      </div>
    </section>
  )
}

function CliPlatformGroup({
  os,
  entries,
}: {
  os: DownloadOS
  entries: readonly DownloadEntry[]
}) {
  if (entries.length === 0) return null
  const heading = OS_HEADINGS[os]
  const snippetEntry = entries[0]
  return (
    <div className={cn('flex flex-col gap-4')}>
      <h3 className="text-foreground text-lg font-semibold select-none @lg:text-xl">
        {heading}
      </h3>
      <CliInstallSnippet entry={snippetEntry} />
      <div className="grid gap-4 @lg:grid-cols-2">
        {entries.map((entry) => (
          <ArchTile key={entry.filename} entry={entry} />
        ))}
      </div>
    </div>
  )
}
