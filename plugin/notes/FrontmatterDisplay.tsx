import { useMemo } from 'react'

import type { Frontmatter } from './frontmatter.js'
import { getFrontmatterTags, stripWikiLinks } from './frontmatter.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LuTag, LuUser, LuCalendar, LuExternalLink } from 'react-icons/lu'

interface FrontmatterDisplayProps {
  frontmatter: Frontmatter
  className?: string
  onTagClick?: (tag: string | undefined) => void
  onStatusClick?: (status: string | undefined) => void
}

// statusTone picks the badge colors for a frontmatter status.
function statusTone(status: string): string {
  if (status === 'done' || status === 'complete') {
    return 'bg-green-500/10 text-green-400'
  }
  if (status === 'in-progress') return 'bg-yellow-500/10 text-yellow-400'
  return 'bg-muted text-muted-foreground'
}

// StatusBadge renders the status, as a filter button when onClick is set.
function StatusBadge({
  status,
  onClick,
}: {
  status: string
  onClick?: (status: string) => void
}) {
  const className = cn(
    'rounded-full px-2 py-0.5 text-xs font-medium',
    statusTone(status),
  )
  if (!onClick) return <span className={className}>{status}</span>

  return (
    <button
      type="button"
      className={cn(className, 'hover:opacity-80')}
      onClick={() => onClick(status)}
      title={`Filter by status: ${status}`}
    >
      {status}
    </button>
  )
}

// FrontmatterByline renders the authors, date, and source link.
function FrontmatterByline({
  authors,
  frontmatter,
}: {
  authors: string[]
  frontmatter: Frontmatter
}) {
  const date = frontmatter.published ?? frontmatter.created

  return (
    <>
      {authors.length > 0 && (
        <span className="text-muted-foreground flex items-center gap-1 text-xs">
          <LuUser className="size-2.5" />
          {authors.join(', ')}
        </span>
      )}

      {date && (
        <span className="text-muted-foreground flex items-center gap-1 text-xs">
          <LuCalendar className="size-2.5" />
          {date}
        </span>
      )}

      {frontmatter.url && (
        <a
          href={frontmatter.url}
          target="_blank"
          rel="noopener noreferrer"
          className="text-brand flex items-center gap-1 text-xs hover:underline"
        >
          <LuExternalLink className="size-2.5" />
          source
        </a>
      )}
    </>
  )
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

  const hasContent = [
    tags,
    categories,
    authors,
    frontmatter.status,
    frontmatter.created,
    frontmatter.published,
    frontmatter.url,
  ].some((value) => (Array.isArray(value) ? value.length > 0 : value))

  if (!hasContent) return null

  return (
    <div
      className={cn(
        'flex flex-wrap items-center gap-2 border-b border-border px-4 py-2',
        className,
      )}
    >
      {frontmatter.status && (
        <StatusBadge status={frontmatter.status} onClick={onStatusClick} />
      )}

      {tags.map((tag) => (
        <button
          key={tag}
          type="button"
          className="bg-brand/10 text-brand flex items-center gap-1 rounded-full px-2 py-0.5 text-xs hover:opacity-80"
          onClick={() => onTagClick?.(tag)}
          title={`Filter by tag: ${tag}`}
        >
          <LuTag className="size-2.5" />
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

      <FrontmatterByline authors={authors} frontmatter={frontmatter} />
    </div>
  )
}

export default FrontmatterDisplay
