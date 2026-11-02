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
import { cn } from '@s4wave/web/style/utils.js'
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
const toolbarActionBase =
  'flex h-11 min-w-11 items-center justify-center gap-1.5 rounded-md px-2 text-xs transition-colors @2xl:h-8 @2xl:min-w-0 @2xl:justify-start'

// toolbarActionIdle is the resting color treatment shared by the header actions.
const toolbarActionIdle =
  'text-foreground-alt/40 hover:text-foreground-alt hover:bg-foreground/5'

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
      <div className="mb-6 flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
        <div className="text-foreground-alt/50 flex min-w-0 flex-1 items-center gap-2 text-xs">
          <span className="hidden @2xl:inline">
            {siteLabels.get(doc.site) ?? doc.site}
          </span>
          <span className="text-foreground-alt/30 hidden @2xl:inline">/</span>
          <span className="hidden @2xl:inline">
            {getSectionLabel(doc.site, doc.section)}
          </span>
          <span className="text-foreground-alt/30 hidden @2xl:inline">/</span>
          <span className="text-foreground-alt hidden truncate @2xl:inline">
            {doc.title}
          </span>
        </div>

        <div className="flex shrink-0 items-center gap-1">
          <button
            type="button"
            onClick={handleCopyMarkdown}
            className={cn(
              toolbarActionBase,
              copied ? 'text-brand' : toolbarActionIdle,
            )}
            title="Copy as Markdown"
            aria-label="Copy page as Markdown"
          >
            {copied ? (
              <LuCheck sx={stylexStyles.rule1} />
            ) : (
              <LuCopy sx={stylexStyles.rule1} />
            )}
            <span className="hidden @2xl:inline">
              {copied ? 'Copied' : 'Copy MD'}
            </span>
          </button>

          <ExternalLink
            href={rawGitHubUrl}
            className={cn(toolbarActionBase, toolbarActionIdle)}
            title="Open raw Markdown on GitHub"
            aria-label="Open raw Markdown on GitHub"
          >
            <LuFileText sx={stylexStyles.rule1} />
            <span className="hidden @2xl:inline">Open MD</span>
          </ExternalLink>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                className={cn(toolbarActionBase, toolbarActionIdle)}
                title="Open in AI"
                aria-label="Open page in an AI assistant"
              >
                <LuSparkles sx={stylexStyles.rule1} />
                <span className="hidden @2xl:inline">AI</span>
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
      <header className="mb-8">
        <h1 className="text-foreground mb-3 text-2xl leading-snug font-semibold tracking-tight @2xl:text-3xl @2xl:leading-snug">
          {doc.title}
        </h1>
        <p className="text-foreground-alt text-sm leading-relaxed">
          {doc.summary}
        </p>
      </header>

      {/* Page body */}
      <div className="docs-prose">
        <Markdown options={docsMarkdownOverrides}>{doc.body}</Markdown>
      </div>

      {/* Previous / Next navigation */}
      {(prevDoc || nextDoc) && (
        <nav className="mt-12 grid grid-cols-1 gap-3 @sm:grid-cols-2 @sm:gap-4">
          {prevDoc ? (
            <button
              type="button"
              onClick={navigatePrev}
              sx={[stylexStyles2.rule1, groupMarker2]}
            >
              <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                <LuArrowLeft sx={stylexStyles.rule3} />
                Previous
              </span>
              <span className="text-foreground group-hover:text-brand text-sm font-medium transition-colors duration-200">
                {prevDoc.title}
              </span>
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
              <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                Next
                <LuArrowRight sx={stylexStyles.rule4} />
              </span>
              <span className="text-foreground group-hover:text-brand text-sm font-medium transition-colors duration-200">
                {nextDoc.title}
              </span>
            </button>
          ) : (
            <div />
          )}
        </nav>
      )}
    </article>
  )
}
