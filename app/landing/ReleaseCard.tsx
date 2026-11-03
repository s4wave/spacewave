import Markdown from 'markdown-to-jsx'
import type { AnchorHTMLAttributes } from 'react'
import { LuGithub } from '@s4wave/web/images/icons.js'

import { Badge } from '@s4wave/web/ui/badge.js'

import type {
  Release,
  ChangeEntry,
} from '@s4wave/core/changelog/changelog.pb.js'

import { ExternalLink } from './ExternalLink.js'
import { useStaticHref } from '../prerender/StaticContext.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
  rule2: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    '--tw-shadow': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover':
          '0 4px 6px -1px var(--tw-shadow-color, #0000001a), 0 2px 4px -2px var(--tw-shadow-color, #0000001a)',
      },
    },
    boxShadow: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover':
          'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      },
    },
    '--tw-shadow-color': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent) var(--tw-shadow-alpha), transparent)`,
      },
    },
  },
  rule5: {
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
  },
  rule6: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule7: {
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
  },
  rule8: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    flexShrink: 0,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { marginBottom: `calc(${themeLayout['--spacing']} * 2)` },
})

const markdownOptions = {
  forceInline: true,
  overrides: {
    a: {
      component: function ChangelogLink(
        props: Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'className'> & {
          sx?: stylex.StyleXStyles
        },
      ) {
        const { sx, ...rest } = props
        return <ExternalLink {...rest} sx={[changelogLinkStyles.link, sx]} />
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
    <div sx={stylexStyles4.rule1}>
      <Badge variant="release" sx={stylexStyles.rule1}>
        {label}
      </Badge>
      <ul sx={stylexStyles4.rule2}>
        {entries.map((entry) => (
          <li
            key={entry.descriptionMarkdown || entry.description}
            sx={stylexStyles4.rule3}
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
    <div id={`v${release.version}`} sx={stylexStyles4.rule4}>
      <div sx={stylexStyles4.rule5}>
        <h2 sx={stylexStyles4.rule6}>
          {linkToDetail ? (
            <a href={detailHref} sx={stylexStyles4.rule7}>
              v{release.version}
            </a>
          ) : (
            <>v{release.version}</>
          )}
        </h2>
        {release.releaseUrl && (
          <ExternalLink
            href={release.releaseUrl}
            sx={stylexStyles3.rule1}
            title="View release"
          >
            <LuGithub sx={stylexStyles2.rule1} />
          </ExternalLink>
        )}
      </div>
      {release.date && <p sx={stylexStyles4.rule8}>{release.date}</p>}
      {release.summary && (
        <div sx={stylexStyles4.rule9}>
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

// changelogLinkStyles keeps link decoration at the markdown anchor owner.
const changelogLinkStyles = stylex.create({
  link: {
    color: {
      default: 'var(--color-foreground)',
      '@media (hover:hover)': { default: null, ':hover': 'var(--color-white)' },
    },
    textDecorationLine: 'underline',
    textDecorationColor: {
      default: 'color-mix(in oklab, var(--color-white) 20%, transparent)',
      '@media (hover:hover)': {
        default: null,
        ':hover': 'color-mix(in oklab, var(--color-white) 60%, transparent)',
      },
    },
    textUnderlineOffset: '3px',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction:
      'var(--tw-ease, var(--default-transition-timing-function))',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})
