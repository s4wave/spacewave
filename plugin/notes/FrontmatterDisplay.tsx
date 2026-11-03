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
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { utilityState as themeUtilityState } from '../../web/style/tokens.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    borderRadius: '3.40282e38px',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule2: {
    borderRadius: '3.40282e38px',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
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
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-brand'],
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-brand'],
    opacity: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 0.8 },
    },
  },
  rule4: {
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-muted'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
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
