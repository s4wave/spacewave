import { useMemo } from 'react'

import type { Frontmatter } from './frontmatter.js'
import { getFrontmatterTags, stripWikiLinks } from './frontmatter.js'

import {
  LuTag,
  LuUser,
  LuCalendar,
  LuExternalLink,
} from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '@s4wave/web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
} from '@s4wave/web/style/tokens.stylex.js'

import { utilityState as themeUtilityState } from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: {
    borderColor: themeColors['--color-border'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexWrap: 'wrap',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 0.8 },
    },
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-green-500']} 10%, transparent)`,
    color: themeColors['--color-green-400'],
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 10%, transparent)`,
    color: themeColors['--color-yellow-400'],
  },
  rule3: {
    backgroundColor: themeColors['--color-muted'],
    color: themeColors['--color-muted-foreground'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    color: themeColors['--color-brand'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
  rule3: {
    borderRadius: '3.40282e38px',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 0.8 },
    },
  },
  rule4: {
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: themeColors['--color-muted'],
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
})

interface FrontmatterDisplayProps {
  frontmatter: Frontmatter
  sx?: stylex.StyleXStyles
  className?: never
  onTagClick?: (tag: string | undefined) => void
  onStatusClick?: (status: string | undefined) => void
}

// statusTone picks the badge colors for a frontmatter status.
function statusTone(status: string): stylex.StyleXStyles {
  if (status === 'done' || status === 'complete') {
    return stylexStyles3.rule1
  }
  if (status === 'in-progress') return stylexStyles3.rule2
  return stylexStyles3.rule3
}

// StatusBadge renders the status, as a filter button when onClick is set.
function StatusBadge({
  status,
  onClick,
}: {
  status: string
  onClick?: (status: string) => void
}) {
  const badgeSx = statusTone(status)
  if (!onClick)
    return <span sx={[[stylexStyles4.rule1], [badgeSx]]}>{status}</span>

  return (
    <button
      type="button"
      sx={[[stylexStyles4.rule2], [badgeSx]]}
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
        <span sx={stylexStyles2.rule1}>
          <LuUser sx={stylexStyles.rule1} />
          {authors.join(', ')}
        </span>
      )}

      {date && (
        <span sx={stylexStyles2.rule1}>
          <LuCalendar sx={stylexStyles.rule1} />
          {date}
        </span>
      )}

      {frontmatter.url && (
        <a
          href={frontmatter.url}
          target="_blank"
          rel="noopener noreferrer"
          sx={stylexStyles2.rule2}
        >
          <LuExternalLink sx={stylexStyles.rule1} />
          source
        </a>
      )}
    </>
  )
}

// FrontmatterDisplay renders parsed frontmatter as structured UI.
function FrontmatterDisplay({
  frontmatter,
  sx,
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
    <div sx={[stylexStyles5.rule1, sx]}>
      {frontmatter.status && (
        <StatusBadge status={frontmatter.status} onClick={onStatusClick} />
      )}

      {tags.map((tag) => (
        <button
          key={tag}
          type="button"
          sx={stylexStyles2.rule3}
          onClick={() => onTagClick?.(tag)}
          title={`Filter by tag: ${tag}`}
        >
          <LuTag sx={stylexStyles.rule1} />
          {tag}
        </button>
      ))}

      {categories.map((cat) => (
        <span key={cat} sx={stylexStyles2.rule4}>
          {cat}
        </span>
      ))}

      <FrontmatterByline authors={authors} frontmatter={frontmatter} />
    </div>
  )
}

export default FrontmatterDisplay
