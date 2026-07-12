import { useCallback, useMemo, useRef, useState } from 'react'
import { LuArrowLeft, LuChevronDown } from 'react-icons/lu'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { LegalFooter } from './LegalFooter.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { ReleaseCard } from './ReleaseCard.js'
import { useLandingBackNavigation } from './useLandingBackNavigation.js'

export const metadata = {
  title: 'Changelog - Spacewave',
  description: 'See what is new in Spacewave.',
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
    // Version ids contain dots (e.g. v0.53.1), so escape them before
    // building the id selector; a raw `#v0.53.1` is an invalid selector.
    const el = scrollRef.current?.querySelector(`#${CSS.escape(`v${version}`)}`)
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
      {/* Back button */}
      <div className="relative z-10 px-4 pt-6 @lg:px-8">
        <button
          onClick={goBack}
          className="text-foreground-alt hover:text-foreground flex cursor-pointer items-center gap-2 text-sm transition-colors"
        >
          <LuArrowLeft className="size-4" />
          Back
        </button>
      </div>

      {/* Hero */}
      <header className="relative z-10 mx-auto w-full max-w-4xl px-4 pt-14 pb-8 text-center @lg:px-8 @lg:pt-20 @lg:pb-10">
        <h1 className="text-foreground mb-6 text-4xl font-semibold tracking-tight @lg:text-5xl">
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
                  'size-4 transition-transform',
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
            <ReleaseCard key={release.version} release={release} linkToDetail />
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
