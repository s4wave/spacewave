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
} from 'react-icons/lu'
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
          <span className="text-foreground-alt truncate">{doc.title}</span>
        </div>

        <div className="flex shrink-0 items-center gap-1">
          <button
            onClick={handleCopyMarkdown}
            className={cn(
              toolbarActionBase,
              copied ? 'text-brand' : toolbarActionIdle,
            )}
            title="Copy as Markdown"
            aria-label="Copy page as Markdown"
          >
            {copied ? (
              <LuCheck className="size-3" />
            ) : (
              <LuCopy className="size-3" />
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
            <LuFileText className="size-3" />
            <span className="hidden @2xl:inline">Open MD</span>
          </ExternalLink>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                className={cn(toolbarActionBase, toolbarActionIdle)}
                title="Open in AI"
                aria-label="Open page in an AI assistant"
              >
                <LuSparkles className="size-3" />
                <span className="hidden @2xl:inline">AI</span>
                <LuChevronDown className="size-2.5" />
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
              onClick={navigatePrev}
              className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-start gap-1.5 rounded-xl border p-4 text-left transition duration-200 @lg:p-5"
            >
              <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                <LuArrowLeft className="size-3 transition-transform duration-200 group-hover:-translate-x-0.5" />
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
              onClick={navigateNext}
              className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-end gap-1.5 rounded-xl border p-4 text-right transition duration-200 @lg:p-5"
            >
              <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                Next
                <LuArrowRight className="size-3 transition-transform duration-200 group-hover:translate-x-0.5" />
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
