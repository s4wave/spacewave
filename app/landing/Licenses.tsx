import { useState, useMemo, useCallback } from 'react'
import {
  LuScale,
  LuSearch,
  LuChevronDown,
  LuChevronRight,
} from 'react-icons/lu'
import {
  licenseEntries,
  licenseBases,
  licenseStats,
  reconstructText,
  type AnnotatedLicenseEntry,
} from '@s4wave/app/licenses/data.js'
import { LegalPageLayout } from './LegalPageLayout.js'

export const metadata = {
  title: 'Open Source Licenses - Spacewave',
  description: 'Third-party open source software licenses used in Spacewave.',
  canonicalPath: '/licenses',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

function getLicenseEntryKey(entry: AnnotatedLicenseEntry): string {
  return `${entry.name}:${entry.version}:${entry.spdx}:${entry.source}:${entry.repo ?? 'local'}`
}

function SourceBadge({ source }: { source: string }) {
  const label =
    source === 'both' ? 'JS + Go'
    : source === 'js' ? 'JS'
    : 'Go'
  return (
    <span className="bg-foreground/8 text-foreground-alt rounded px-1.5 py-0.5 text-[10px] font-medium uppercase">
      {label}
    </span>
  )
}

function SpdxBadge({ spdx }: { spdx: string }) {
  return (
    <span className="border-foreground/10 text-foreground-alt rounded border px-1.5 py-0.5 text-[10px] font-medium">
      {spdx}
    </span>
  )
}

function LicenseGroup({
  spdx,
  entries,
  baseText,
}: {
  spdx: string
  entries: AnnotatedLicenseEntry[]
  baseText?: string
}) {
  const [expanded, setExpanded] = useState<string | null>(null)
  const [showBase, setShowBase] = useState(false)

  const toggleBase = useCallback(() => setShowBase((v) => !v), [])
  const toggleEntry = useCallback(
    (key: string) => setExpanded((v) => (v === key ? null : key)),
    [],
  )

  return (
    <div className="border-foreground/8 bg-background-card/50 rounded-lg border backdrop-blur-sm">
      <div className="flex items-center gap-3 px-5 py-4">
        <SpdxBadge spdx={spdx} />
        <span className="text-foreground text-sm font-medium">{spdx}</span>
        <span className="text-foreground-alt/60 text-xs">
          {entries.length} {entries.length === 1 ? 'package' : 'packages'}
        </span>
        {baseText && (
          <button
            onClick={toggleBase}
            className="text-foreground-alt/60 hover:text-foreground-alt ml-auto flex cursor-pointer items-center gap-1 text-xs transition-colors"
          >
            {showBase ? 'Hide' : 'Show'} license text
            {showBase ?
              <LuChevronDown className="h-3 w-3" />
            : <LuChevronRight className="h-3 w-3" />}
          </button>
        )}
      </div>

      {showBase && baseText && (
        <div className="border-foreground/8 border-t px-5 py-4">
          <pre className="text-foreground-alt/70 text-xs leading-relaxed whitespace-pre-wrap">
            {baseText}
          </pre>
        </div>
      )}

      <div className="border-foreground/8 border-t">
        {entries.map((entry) => {
          const entryKey = getLicenseEntryKey(entry)
          const isExpanded = expanded === entryKey
          const hasCustomText = !!entry.fullText
          const hasCopyright = !!entry.copyrightNotice
          const buttonLabel = `${isExpanded ? 'Hide' : 'Show'} details for ${entry.name} ${entry.version}`

          return (
            <div
              key={entryKey}
              className="border-foreground/5 border-b px-5 py-3 last:border-b-0"
            >
              <div className="flex items-center gap-2">
                {(hasCustomText || hasCopyright) && (
                  <button
                    onClick={() => toggleEntry(entryKey)}
                    aria-expanded={isExpanded}
                    aria-label={buttonLabel}
                    className="text-foreground-alt/60 hover:text-foreground-alt cursor-pointer transition-colors"
                  >
                    {isExpanded ?
                      <LuChevronDown className="h-3.5 w-3.5" />
                    : <LuChevronRight className="h-3.5 w-3.5" />}
                  </button>
                )}
                <span className="text-foreground text-sm">
                  {entry.repo ?
                    <a
                      href={entry.repo}
                      className="hover:text-brand transition-colors hover:underline"
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {entry.name}
                    </a>
                  : entry.name}
                </span>
                <span className="text-foreground-alt/50 text-xs">
                  {entry.version}
                </span>
                <SourceBadge source={entry.source} />
              </div>

              {isExpanded && (
                <div className="mt-2 pl-6">
                  {hasCopyright && (
                    <p className="text-foreground-alt/70 mb-1 text-xs">
                      {entry.copyrightNotice}
                    </p>
                  )}
                  {hasCustomText && (
                    <pre className="text-foreground-alt/60 mt-2 max-h-64 overflow-y-auto text-[11px] leading-relaxed whitespace-pre-wrap">
                      {reconstructText(entry)}
                    </pre>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// Licenses renders the open source licenses page.
export function Licenses() {
  const [filter, setFilter] = useState('')
  const stats = licenseStats()

  const filtered = useMemo(() => {
    if (!filter) return licenseEntries
    const q = filter.toLowerCase()
    return licenseEntries.filter(
      (e) =>
        e.name.toLowerCase().includes(q) || e.spdx.toLowerCase().includes(q),
    )
  }, [filter])

  const groups = useMemo(() => {
    const map = new Map<string, AnnotatedLicenseEntry[]>()
    for (const entry of filtered) {
      const spdx = entry.spdx
      if (!map.has(spdx)) map.set(spdx, [])
      map.get(spdx)!.push(entry)
    }
    return [...map.entries()]
      .sort((a, b) => b[1].length - a[1].length)
      .map(([spdx, entries]) => ({
        spdx,
        entries: entries.sort((a, b) => {
          const nameDiff = a.name.localeCompare(b.name)
          if (nameDiff !== 0) return nameDiff
          return a.version.localeCompare(b.version)
        }),
      }))
  }, [filtered])

  const onFilterChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => setFilter(e.target.value),
    [],
  )

  return (
    <LegalPageLayout
      icon={<LuScale className="h-10 w-10" />}
      title="Open Source Licenses"
      subtitle="Spacewave is built with open source software. This page lists all third-party dependencies and their licenses."
    >
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-14 @lg:px-8 @lg:pb-16">
        <div className="mb-8 flex flex-col gap-4 @md:flex-row @md:items-center @md:justify-between">
          <div className="text-foreground-alt/60 flex flex-wrap gap-3 text-xs">
            <span>{stats.total} packages</span>
            <span>{stats.jsCount} JavaScript</span>
            <span>{stats.goCount} Go</span>
            <span>{stats.uniqueLicenses} license types</span>
          </div>

          <div className="relative">
            <LuSearch className="text-foreground-alt/40 pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2" />
            <input
              type="text"
              placeholder="Filter packages..."
              value={filter}
              onChange={onFilterChange}
              className="border-foreground/10 bg-background-card/50 text-foreground placeholder:text-foreground-alt/40 w-full rounded-md border py-1.5 pr-3 pl-8 text-sm backdrop-blur-sm focus:ring-1 focus:ring-white/20 focus:outline-none @md:w-64"
            />
          </div>
        </div>

        <div className="space-y-6">
          {groups.map(({ spdx, entries }) => (
            <LicenseGroup
              key={spdx}
              spdx={spdx}
              entries={entries}
              baseText={licenseBases[spdx]}
            />
          ))}
        </div>

        {filtered.length === 0 && (
          <p className="text-foreground-alt/50 py-12 text-center text-sm">
            No packages match &ldquo;{filter}&rdquo;
          </p>
        )}
      </section>
    </LegalPageLayout>
  )
}
