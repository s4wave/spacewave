import Markdown from 'markdown-to-jsx'
import { useCallback, useMemo, useRef, useState } from 'react'
import type { AnchorHTMLAttributes } from 'react'
import { LuArrowLeft, LuChevronDown, LuGithub } from 'react-icons/lu'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { Badge } from '@s4wave/web/ui/badge.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { LegalFooter } from './LegalFooter.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useLandingBackNavigation } from './useLandingBackNavigation.js'
import type {
  Release,
  ChangeEntry,
} from '@s4wave/core/changelog/changelog.pb.js'

export const metadata = {
  title: 'Changelog - Spacewave',
  description: 'See what is new in Spacewave.',
}

const markdownOptions = {
  forceInline: true,
  overrides: {
    a: {
      component: function ChangelogLink(
        props: AnchorHTMLAttributes<HTMLAnchorElement>,
      ) {
        const { className, ...rest } = props
        return (
          <a
            {...rest}
            target="_blank"
            rel="noopener noreferrer"
            className={cn(
              'text-foreground underline decoration-white/20 underline-offset-3 transition-colors hover:text-white hover:decoration-white/60',
              className,
            )}
          />
        )
      },
    },
  },
} as const

// CategorySection renders a list of change entries under a badge label.
function CategorySection({
  label,
  entries,
}: {
  label: string
  entries: ChangeEntry[] | undefined
}) {
  if (!entries || entries.length === 0) return null

  return (
    <div className="mt-4">
      <Badge
        variant="outline"
        className="text-foreground-alt border-foreground/15 mb-2 text-xs"
      >
        {label}
      </Badge>
      <ul className="flex flex-col gap-2">
        {entries.map((entry, i) => (
          <li key={i} className="text-foreground-alt text-sm leading-relaxed">
            <Markdown options={markdownOptions}>
              {entry.descriptionMarkdown || entry.description || ''}
            </Markdown>
          </li>
        ))}
      </ul>
    </div>
  )
}

// ReleaseCard renders a single release entry with version, date, summary, and categorized changes.
function ReleaseCard({ release }: { release: Release }) {
  return (
    <div
      id={`v${release.version}`}
      className="border-foreground/8 bg-background-card/50 hover:border-foreground/20 hover:shadow-foreground/5 rounded-lg border p-5 backdrop-blur-sm transition-all duration-300 hover:shadow-md"
    >
      <div className="flex items-start justify-between">
        <h2 className="text-foreground text-lg font-bold">
          v{release.version}
        </h2>
        {release.releaseUrl && (
          <a
            href={release.releaseUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="text-foreground-alt/50 hover:text-foreground shrink-0 transition-colors"
            title="View release"
          >
            <LuGithub className="h-5 w-5" />
          </a>
        )}
      </div>
      {release.date && (
        <p className="text-foreground-alt mt-1 text-sm">{release.date}</p>
      )}
      {release.summary && (
        <div className="text-foreground-alt mt-3 text-sm leading-relaxed">
          <Markdown options={markdownOptions}>
            {release.summaryMarkdown || release.summary}
          </Markdown>
        </div>
      )}
      <CategorySection label="Features" entries={release.features} />
      <CategorySection label="Fixes" entries={release.fixes} />
      <CategorySection label="Improvements" entries={release.improvements} />
      <CategorySection label="Security" entries={release.security} />
    </div>
  )
}

// Changelog renders the changelog landing page.
export function Changelog() {
  const goBack = useLandingBackNavigation()

  const rootResource = useRootResource()
  const changelogResource = useResource(
    rootResource,
    async (root, signal) => root.getChangelog(signal),
    [],
  )
  const releases = useMemo(
    () => changelogResource.value?.releases ?? [],
    [changelogResource.value],
  )

  const [dropdownOpen, setDropdownOpen] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)

  const toggleDropdown = useCallback(() => {
    setDropdownOpen((prev) => !prev)
  }, [])

  const scrollToVersion = useCallback((version: string) => {
    setDropdownOpen(false)
    const el = scrollRef.current?.querySelector(`#v${version}`)
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, [])

  const latestVersion = releases.length > 0 ? releases[0].version : null

  return (
    <div
      ref={scrollRef}
      className="bg-background-landing @container flex w-full flex-1 flex-col overflow-y-auto"
    >
      <ShootingStars className="pointer-events-none fixed inset-0 opacity-40" />

      {/* Back button */}
      <div className="relative z-10 px-4 pt-6 @lg:px-8">
        <button
          onClick={goBack}
          className="text-foreground-alt hover:text-foreground flex cursor-pointer items-center gap-2 text-sm transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
          Back
        </button>
      </div>

      {/* Hero */}
      <header className="relative z-10 mx-auto w-full max-w-4xl px-4 pt-14 pb-8 text-center @lg:px-8 @lg:pt-20 @lg:pb-10">
        <h1 className="text-foreground mb-6 text-4xl font-bold tracking-tight @lg:text-5xl">
          Changelog
        </h1>
        <p className="text-foreground-alt mx-auto max-w-xl text-base leading-relaxed @lg:text-lg">
          See what is new in Spacewave.
        </p>
      </header>

      {/* Version dropdown */}
      {releases.length > 0 && (
        <div className="relative z-20 mx-auto w-full max-w-4xl px-4 pb-6 @lg:px-8">
          <div className="relative inline-block">
            <button
              onClick={toggleDropdown}
              className={cn(
                'border-foreground/15 bg-background-card/50 text-foreground flex cursor-pointer items-center gap-2 rounded-md border px-4 py-2 text-sm font-medium backdrop-blur-sm transition-colors',
                dropdownOpen && 'border-foreground/25',
              )}
            >
              {latestVersion ? `v${latestVersion}` : 'Versions'}
              <LuChevronDown
                className={cn(
                  'h-4 w-4 transition-transform',
                  dropdownOpen && 'rotate-180',
                )}
              />
            </button>
            {dropdownOpen && (
              <div className="border-foreground/15 bg-background-card absolute left-0 mt-1 max-h-60 w-48 overflow-y-auto rounded-md border shadow-lg backdrop-blur-sm">
                {releases.map((release) => (
                  <button
                    key={release.version}
                    onClick={() => scrollToVersion(release.version ?? '')}
                    className="text-foreground-alt hover:bg-foreground/5 hover:text-foreground w-full cursor-pointer px-4 py-2 text-left text-sm transition-colors"
                  >
                    v{release.version}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Release cards */}
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-14 @lg:px-8 @lg:pb-16">
        <div className="flex flex-col gap-6">
          {releases.map((release) => (
            <ReleaseCard key={release.version} release={release} />
          ))}
        </div>
        {changelogResource.loading && (
          <div className="mx-auto mt-8 w-full max-w-sm">
            <LoadingCard
              view={{
                state: 'active',
                title: 'Loading changelog',
                detail: 'Fetching the latest Spacewave releases.',
              }}
            />
          </div>
        )}
        {!changelogResource.loading && releases.length === 0 && (
          <p className="text-foreground-alt mt-8 text-center text-sm">
            No releases yet.
          </p>
        )}
      </section>

      <LegalFooter />
    </div>
  )
}
