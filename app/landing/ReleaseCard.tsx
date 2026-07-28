import Markdown from 'markdown-to-jsx'
import type { AnchorHTMLAttributes } from 'react'
import { LuGithub } from 'react-icons/lu'

import { Badge } from '@s4wave/web/ui/badge.js'
import { cn } from '@s4wave/web/style/utils.js'
import type {
  Release,
  ChangeEntry,
} from '@s4wave/core/changelog/changelog.pb.js'

import { ExternalLink } from './ExternalLink.js'
import { useStaticHref } from '../prerender/StaticContext.js'

const markdownOptions = {
  forceInline: true,
  overrides: {
    a: {
      component: function ChangelogLink(
        props: AnchorHTMLAttributes<HTMLAnchorElement>,
      ) {
        const { className, ...rest } = props
        return (
          <ExternalLink
            {...rest}
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
        {entries.map((entry) => (
          <li
            key={entry.descriptionMarkdown || entry.description}
            className="text-foreground-alt text-sm leading-relaxed"
          >
            <Markdown options={markdownOptions}>
              {entry.descriptionMarkdown || entry.description || ''}
            </Markdown>
          </li>
        ))}
      </ul>
    </div>
  )
}

// ReleaseCard renders a single release entry with version, date, summary, and
// categorized changes. When linkToDetail is set the version heading links to
// the release's own page (/changelog/vX.Y.Z), crawlable in static mode.
export function ReleaseCard({
  release,
  linkToDetail = false,
}: {
  release: Release
  linkToDetail?: boolean
}) {
  const detailHref = useStaticHref(`/changelog/v${release.version ?? ''}`)

  return (
    <div
      id={`v${release.version}`}
      className="border-foreground/8 bg-background-card/50 hover:border-foreground/20 hover:shadow-foreground/5 rounded-lg border p-5 backdrop-blur-sm transition duration-300 hover:shadow-md"
    >
      <div className="flex items-start justify-between">
        <h2 className="text-foreground text-lg font-semibold">
          {linkToDetail ? (
            <a href={detailHref} className="hover:text-brand transition-colors">
              v{release.version}
            </a>
          ) : (
            <>v{release.version}</>
          )}
        </h2>
        {release.releaseUrl && (
          <ExternalLink
            href={release.releaseUrl}
            className="text-foreground-alt/50 hover:text-foreground shrink-0 transition-colors"
            title="View release"
          >
            <LuGithub className="size-5" />
          </ExternalLink>
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
