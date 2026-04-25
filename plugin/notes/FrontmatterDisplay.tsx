import { useMemo } from 'react'

import type { Frontmatter } from './frontmatter.js'
import {
  getFrontmatterTags,
  stripWikiLinks,
} from './frontmatter.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LuTag, LuUser, LuCalendar, LuExternalLink } from 'react-icons/lu'

interface FrontmatterDisplayProps {
  frontmatter: Frontmatter
  className?: string
  onTagClick?: (tag: string | undefined) => void
  onStatusClick?: (status: string | undefined) => void
}

// FrontmatterDisplay renders parsed frontmatter as structured UI.
function FrontmatterDisplay({
  frontmatter,
  className,
  onTagClick,
  onStatusClick,
}: FrontmatterDisplayProps) {
  const tags = useMemo(() => getFrontmatterTags(frontmatter), [frontmatter])

  const categories = useMemo(
    () => (frontmatter.categories ?? []).map(stripWikiLinks),
    [frontmatter.categories],
  )

  const authors = useMemo(
    () => (frontmatter.author ?? []).map(stripWikiLinks),
    [frontmatter.author],
  )

  const hasContent =
    tags.length > 0 ||
    categories.length > 0 ||
    authors.length > 0 ||
    frontmatter.status ||
    frontmatter.created ||
    frontmatter.published ||
    frontmatter.url

  if (!hasContent) return null

  return (
    <div
      className={cn(
        'flex flex-wrap items-center gap-2 border-b border-border px-4 py-2',
        className,
      )}
    >
      {frontmatter.status &&
        (onStatusClick ?
          <button
            type="button"
            className={cn(
              'rounded-full px-2 py-0.5 text-xs font-medium hover:opacity-80',
              frontmatter.status === 'done' || frontmatter.status === 'complete'
                ? 'bg-green-500/10 text-green-400'
                : frontmatter.status === 'in-progress'
                  ? 'bg-yellow-500/10 text-yellow-400'
                  : 'bg-muted text-muted-foreground',
            )}
            onClick={() => onStatusClick(frontmatter.status)}
            title={`Filter by status: ${frontmatter.status}`}
          >
            {frontmatter.status}
          </button>
        : <span
            className={cn(
              'rounded-full px-2 py-0.5 text-xs font-medium',
              frontmatter.status === 'done' || frontmatter.status === 'complete'
                ? 'bg-green-500/10 text-green-400'
                : frontmatter.status === 'in-progress'
                  ? 'bg-yellow-500/10 text-yellow-400'
                  : 'bg-muted text-muted-foreground',
            )}
          >
            {frontmatter.status}
          </span>)}

      {tags.map((tag) => (
        <button
          key={tag}
          type="button"
          className="bg-brand/10 text-brand flex items-center gap-1 rounded-full px-2 py-0.5 text-xs hover:opacity-80"
          onClick={() => onTagClick?.(tag)}
          title={`Filter by tag: ${tag}`}
        >
          <LuTag className="h-2.5 w-2.5" />
          {tag}
        </button>
      ))}

      {categories.map((cat) => (
        <span
          key={cat}
          className="bg-muted text-muted-foreground rounded-full px-2 py-0.5 text-xs"
        >
          {cat}
        </span>
      ))}

      {authors.length > 0 && (
        <span className="text-muted-foreground flex items-center gap-1 text-xs">
          <LuUser className="h-2.5 w-2.5" />
          {authors.join(', ')}
        </span>
      )}

      {(frontmatter.created || frontmatter.published) && (
        <span className="text-muted-foreground flex items-center gap-1 text-xs">
          <LuCalendar className="h-2.5 w-2.5" />
          {frontmatter.published ?? frontmatter.created}
        </span>
      )}

      {frontmatter.url && (
        <a
          href={frontmatter.url}
          target="_blank"
          rel="noopener noreferrer"
          className="text-brand flex items-center gap-1 text-xs hover:underline"
        >
          <LuExternalLink className="h-2.5 w-2.5" />
          source
        </a>
      )}
    </div>
  )
}

export default FrontmatterDisplay
