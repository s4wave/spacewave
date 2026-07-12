import { LuArrowLeft } from 'react-icons/lu'

import type { Release } from '@s4wave/core/changelog/changelog.pb.js'

import { LegalFooter } from './LegalFooter.js'
import { ReleaseCard } from './ReleaseCard.js'
import { ChangelogReleaseDownloads } from './ChangelogReleaseDownloads.js'
import { useLandingBackNavigation } from './useLandingBackNavigation.js'

// ChangelogReleasePage renders a single release's notes plus the desktop-app
// download options for that version.
export function ChangelogReleasePage({ release }: { release: Release }) {
  const goBack = useLandingBackNavigation()

  return (
    <div className="bg-background-landing @container flex w-full flex-1 flex-col overflow-y-auto">
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
        <h1 className="text-foreground mb-4 text-4xl font-semibold tracking-tight @lg:text-5xl">
          Spacewave v{release.version}
        </h1>
        {release.date && (
          <p className="text-foreground-alt text-base @lg:text-lg">
            Released {release.date}
          </p>
        )}
      </header>

      <section className="relative z-10 mx-auto flex w-full max-w-4xl flex-col gap-12 px-4 pb-14 @lg:px-8 @lg:pb-16">
        <ReleaseCard release={release} />
        <ChangelogReleaseDownloads version={release.version ?? ''} />
      </section>

      <LegalFooter />
    </div>
  )
}
