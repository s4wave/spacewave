import { ArchTile } from './ArchTile.js'
import type { DownloadEntry, DownloadOS } from './manifest.js'

interface PlatformSectionProps {
  os: DownloadOS
  entries: readonly DownloadEntry[]
}

const OS_HEADINGS: Record<DownloadOS, string> = {
  macos: 'macOS',
  windows: 'Windows',
  linux: 'Linux',
}

// PlatformSection groups per-architecture download tiles under a single OS
// heading. Windows entries carrying unsigned: true surface an "Interim
// unsigned build" caption under the heading; the SmartScreen bypass details
// render elsewhere (WindowsBypassNotice).
export function PlatformSection({ os, entries }: PlatformSectionProps) {
  const heading = OS_HEADINGS[os]
  const isInterimUnsigned = os === 'windows' && entries.some((e) => e.unsigned)

  return (
    <section className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <h2 className="text-foreground text-2xl font-bold select-none @lg:text-3xl">
          {heading}
        </h2>
        {isInterimUnsigned && (
          <p className="text-foreground-alt text-xs select-none">
            Interim unsigned build
          </p>
        )}
      </div>
      <div className="grid gap-4 @lg:grid-cols-2">
        {entries.map((entry) => (
          <ArchTile key={entry.filename} entry={entry} />
        ))}
      </div>
    </section>
  )
}
