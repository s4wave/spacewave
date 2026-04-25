import { useState, useCallback, useEffect, useMemo, useRef } from 'react'
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
import { PreBlock } from './CodeBlock.js'
import { MarkdownLink } from './MarkdownLink.js'
import { sectionDefs, siteDefs } from './sections.js'
import type { DocPage as DocPageType } from './types.js'
import './docs-prose.css'

// DocsPageProps defines the props for DocsPage.
export interface DocsPageProps {
  doc: DocPageType
  prevDoc?: DocPageType
  nextDoc?: DocPageType
}

// sectionLabels maps section IDs to display labels, derived from sectionDefs.
const sectionLabels = new Map(sectionDefs.map((s) => [s.id, s.label]))
// siteLabels maps site IDs to display labels, derived from siteDefs.
const siteLabels = new Map(siteDefs.map((s) => [s.id, s.label]))

// markdownOverrides configures markdown-to-jsx for code blocks and internal links.
const markdownOverrides = {
  overrides: {
    a: { component: MarkdownLink },
    pre: { component: PreBlock },
  },
}

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
    navigator.clipboard.writeText(doc.body)
    setCopied(true)
    clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopied(false), 1500)
  }, [doc.body])

  const rawGitHubUrl = useMemo(
    () =>
      `https://raw.githubusercontent.com/aperturerobotics/alpha/master/app/docs/content/${doc.site}/${doc.section}/${doc.filename}`,
    [doc.site, doc.section, doc.filename],
  )

  const aiPrompt = useMemo(() => {
    return `I'm reading the Spacewave documentation page "${doc.title}".\n\n${doc.body}`
  }, [doc.title, doc.body])

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
      <div className="mb-6 flex items-center justify-between gap-4">
        <div className="text-foreground-alt/50 flex items-center gap-2 text-xs">
          <span>{siteLabels.get(doc.site) ?? doc.site}</span>
          <span className="text-foreground-alt/30">/</span>
          <span>{sectionLabels.get(doc.section) ?? doc.section}</span>
          <span className="text-foreground-alt/30">/</span>
          <span className="text-foreground-alt">{doc.title}</span>
        </div>

        <div className="flex items-center gap-1">
          <button
            onClick={handleCopyMarkdown}
            className={cn(
              'flex items-center gap-1.5 rounded-md px-2 py-1 text-xs transition-colors',
              copied ? 'text-brand' : (
                'text-foreground-alt/40 hover:text-foreground-alt hover:bg-foreground/5'
              ),
            )}
            title="Copy as Markdown"
          >
            {copied ?
              <LuCheck className="h-3 w-3" />
            : <LuCopy className="h-3 w-3" />}
            <span className="hidden @lg:inline">
              {copied ? 'Copied' : 'Copy MD'}
            </span>
          </button>

          <a
            href={rawGitHubUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="text-foreground-alt/40 hover:text-foreground-alt hover:bg-foreground/5 flex items-center gap-1.5 rounded-md px-2 py-1 text-xs transition-colors"
            title="Open raw Markdown on GitHub"
          >
            <LuFileText className="h-3 w-3" />
            <span className="hidden @lg:inline">Open MD</span>
          </a>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                className="text-foreground-alt/40 hover:text-foreground-alt hover:bg-foreground/5 flex items-center gap-1.5 rounded-md px-2 py-1 text-xs transition-colors"
                title="Open in AI"
              >
                <LuSparkles className="h-3 w-3" />
                <span className="hidden @lg:inline">AI</span>
                <LuChevronDown className="h-2.5 w-2.5" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={handleOpenClaude}>
                Open in Claude
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={handleOpenChatGPT}>
                Open in ChatGPT
              </DropdownMenuItem>
              <DropdownMenuItem
                onSelect={() => navigator.clipboard.writeText(rawGitHubUrl)}
              >
                Copy .md URL
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Page header */}
      <header className="mb-8">
        <h1 className="text-foreground mb-3 text-2xl leading-snug font-bold tracking-tight @lg:text-3xl @lg:leading-snug">
          {doc.title}
        </h1>
        <p className="text-foreground-alt text-sm leading-relaxed">
          {doc.summary}
        </p>
      </header>

      {/* Page body */}
      <div className="docs-prose">
        <Markdown options={markdownOverrides}>{doc.body}</Markdown>
      </div>

      {/* Previous / Next navigation */}
      {(prevDoc || nextDoc) && (
        <nav className="mt-12 grid grid-cols-2 gap-4">
          {prevDoc ?
            <button
              onClick={navigatePrev}
              className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-start gap-1.5 rounded-xl border p-5 text-left transition-all duration-200"
            >
              <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                <LuArrowLeft className="h-3 w-3 transition-transform duration-200 group-hover:-translate-x-0.5" />
                Previous
              </span>
              <span className="text-foreground group-hover:text-brand text-sm font-medium transition-colors duration-200">
                {prevDoc.title}
              </span>
            </button>
          : <div />}

          {nextDoc ?
            <button
              onClick={navigateNext}
              className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-end gap-1.5 rounded-xl border p-5 text-right transition-all duration-200"
            >
              <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                Next
                <LuArrowRight className="h-3 w-3 transition-transform duration-200 group-hover:translate-x-0.5" />
              </span>
              <span className="text-foreground group-hover:text-brand text-sm font-medium transition-colors duration-200">
                {nextDoc.title}
              </span>
            </button>
          : <div />}
        </nav>
      )}
    </article>
  )
}
