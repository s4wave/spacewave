import { useState, useCallback, useEffect, useRef } from 'react'
import Markdown from 'markdown-to-jsx'
import {
  LuArrowLeft,
  LuArrowRight,
  LuCopy,
  LuCheck,
  LuFileText,
  LuSparkles,
  LuChevronDown,
} from '@s4wave/web/images/icons.js'

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { ExternalLink } from '@s4wave/app/landing/ExternalLink.js'
import { docsMarkdownOverrides } from './markdown-overrides.js'
import { getSectionLabel, siteDefs } from './sections.js'
import { getRawMarkdownUrl } from './source-url.js'
import type { DocPage as DocPageType } from './types.js'
import './docs-prose.css'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    columnGap: `calc(${themeLayout['--spacing']} * 3)`,
    rowGap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule3: {
    display: { default: 'none', '@container (min-width: 42rem)': 'inline' },
  },
  rule4: {
    display: { default: 'none', '@container (min-width: 42rem)': 'inline' },
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule5: {
    display: { default: 'none', '@container (min-width: 42rem)': 'inline' },
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule7: {
    display: 'flex',
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    minWidth: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@container (min-width: 42rem)': 0,
    },
    alignItems: 'center',
    justifyContent: {
      default: 'center',
      '@container (min-width: 42rem)': 'flex-start',
    },
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule8: { color: themeColors['--color-brand'] },
  rule9: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule10: { marginBottom: `calc(${themeLayout['--spacing']} * 8)` },
  rule11: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: {
      default: 'var(--text-2xl)',
      '@container (min-width: 42rem)': 'var(--text-3xl)',
    },
    lineHeight: {
      default: 'var(--leading-snug)',
      '@container (min-width: 42rem)': 'var(--leading-snug)',
    },
    '--tw-leading': {
      default: 'var(--leading-snug)',
      '@container (min-width: 42rem)': 'var(--leading-snug)',
    },
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule12: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule13: {
    marginTop: `calc(${themeLayout['--spacing']} * 12)`,
    display: 'grid',
    gridTemplateColumns: {
      default: 'repeat(1, minmax(0, 1fr))',
      '@container (min-width: 24rem)': 'repeat(2, minmax(0, 1fr))',
    },
    gap: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@container (min-width: 24rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
  },
  rule14: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule15: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    minWidth: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@container (min-width: 42rem)': 0,
    },
    alignItems: 'center',
    justifyContent: {
      default: 'center',
      '@container (min-width: 42rem)': 'flex-start',
    },
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    cursor: 'pointer',
    flexDirection: 'column',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    padding: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      },
    },
  },
  rule2: {
    display: 'flex',
    cursor: 'pointer',
    flexDirection: 'column',
    alignItems: 'flex-end',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    padding: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
    textAlign: 'right',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `calc(${themeLayout['--spacing']} * .5)`,
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
})

// DocsPageProps defines the props for DocsPage.
export interface DocsPageProps {
  doc: DocPageType
  prevDoc?: DocPageType
  nextDoc?: DocPageType
}

// siteLabels maps site IDs to display labels, derived from siteDefs.
const siteLabels = new Map(siteDefs.map((s) => [s.id, s.label]))

// toolbarActionBase sizes a header action to a 44px touch target while it is
// icon-only on narrow displays, then relaxes to a compact inline control once
// the @2xl label appears.

// toolbarActionIdle is the resting color treatment shared by the header actions.

// DocsPage renders a single documentation page with markdown content.
export function DocsPage({ doc, prevDoc, nextDoc }: DocsPageProps) {
  const navigate = useNavigate()
  const [copied, setCopied] = useState(false)

  const navigatePrev = useCallback(() => {
    if (prevDoc) navigate({ path: prevDoc.url })
  }, [navigate, prevDoc])

  const navigateNext = useCallback(() => {
    if (nextDoc) navigate({ path: nextDoc.url })
  }, [navigate, nextDoc])

  const copyTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => {
    return () => clearTimeout(copyTimer.current)
  }, [])

  const handleCopyMarkdown = useCallback(() => {
    void navigator.clipboard.writeText(doc.body)
    setCopied(true)
    clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopied(false), 1500)
  }, [doc.body])

  const rawGitHubUrl = getRawMarkdownUrl(doc)

  const handleCopyMarkdownUrl = useCallback(() => {
    void navigator.clipboard.writeText(rawGitHubUrl)
  }, [rawGitHubUrl])

  const aiPrompt = `I'm reading the Spacewave documentation page "${doc.title}".\n\n${doc.body}`

  const handleOpenClaude = useCallback(() => {
    const url = `https://claude.ai/new?q=${encodeURIComponent(aiPrompt)}`
    window.open(url, '_blank')
  }, [aiPrompt])

  const handleOpenChatGPT = useCallback(() => {
    const url = `https://chatgpt.com/?q=${encodeURIComponent(aiPrompt)}`
    window.open(url, '_blank')
  }, [aiPrompt])

  return (
    <article>
      {/* Header bar: breadcrumb + utility actions */}
      <div sx={stylexStyles4.rule1}>
        <div sx={stylexStyles4.rule2}>
          <span sx={stylexStyles4.rule3}>
            {siteLabels.get(doc.site) ?? doc.site}
          </span>
          <span sx={stylexStyles4.rule4}>/</span>
          <span sx={stylexStyles4.rule3}>
            {getSectionLabel(doc.site, doc.section)}
          </span>
          <span sx={stylexStyles4.rule4}>/</span>
          <span sx={stylexStyles4.rule5}>{doc.title}</span>
        </div>

        <div sx={stylexStyles4.rule6}>
          <button
            type="button"
            onClick={handleCopyMarkdown}
            sx={[
              stylexStyles4.rule7,
              copied ? stylexStyles4.rule8 : stylexStyles4.rule9,
            ]}
            title="Copy as Markdown"
            aria-label="Copy page as Markdown"
          >
            {copied ? (
              <LuCheck sx={stylexStyles.rule1} />
            ) : (
              <LuCopy sx={stylexStyles.rule1} />
            )}
            <span sx={stylexStyles4.rule3}>
              {copied ? 'Copied' : 'Copy MD'}
            </span>
          </button>

          <ExternalLink
            href={rawGitHubUrl}
            sx={[stylexStyles3.rule1, stylexStyles3.rule2]}
            title="Open raw Markdown on GitHub"
            aria-label="Open raw Markdown on GitHub"
          >
            <LuFileText sx={stylexStyles.rule1} />
            <span sx={stylexStyles4.rule3}>Open MD</span>
          </ExternalLink>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                sx={[stylexStyles4.rule7, stylexStyles4.rule9]}
                title="Open in AI"
                aria-label="Open page in an AI assistant"
              >
                <LuSparkles sx={stylexStyles.rule1} />
                <span sx={stylexStyles4.rule3}>AI</span>
                <LuChevronDown sx={stylexStyles.rule2} />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={handleOpenClaude}>
                Open in Claude
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={handleOpenChatGPT}>
                Open in ChatGPT
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={handleCopyMarkdownUrl}>
                Copy .md URL
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Page header */}
      <header sx={stylexStyles4.rule10}>
        <h1 sx={stylexStyles4.rule11}>{doc.title}</h1>
        <p sx={stylexStyles4.rule12}>{doc.summary}</p>
      </header>

      {/* Page body */}
      <div
        className={
          /* TODO(stylex): global or unknown class: docs-prose */ 'docs-prose'
        }
      >
        <Markdown options={docsMarkdownOverrides}>{doc.body}</Markdown>
      </div>

      {/* Previous / Next navigation */}
      {(prevDoc || nextDoc) && (
        <nav sx={stylexStyles4.rule13}>
          {prevDoc ? (
            <button
              type="button"
              onClick={navigatePrev}
              sx={[stylexStyles2.rule1, groupMarker2]}
            >
              <span sx={stylexStyles4.rule14}>
                <LuArrowLeft sx={stylexStyles.rule3} />
                Previous
              </span>
              <span sx={stylexStyles4.rule15}>{prevDoc.title}</span>
            </button>
          ) : (
            <div />
          )}

          {nextDoc ? (
            <button
              type="button"
              onClick={navigateNext}
              sx={[stylexStyles2.rule2, groupMarker2]}
            >
              <span sx={stylexStyles4.rule14}>
                Next
                <LuArrowRight sx={stylexStyles.rule4} />
              </span>
              <span sx={stylexStyles4.rule15}>{nextDoc.title}</span>
            </button>
          ) : (
            <div />
          )}
        </nav>
      )}
    </article>
  )
}
