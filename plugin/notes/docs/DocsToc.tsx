import { useCallback, useMemo, useRef } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { LuList } from 'react-icons/lu'

import type { NoteFileFormat } from '../note-files.js'

interface TocHeading {
  level: number
  text: string
  id: string
}

interface DocsTocProps {
  content: string
  format: NoteFileFormat
}

// parseHeadings extracts h1-h4 headings from Markdown or Org content.
function parseHeadings(content: string, format: NoteFileFormat): TocHeading[] {
  const headings: TocHeading[] = []
  const regex =
    format === 'org' ? /^(\*{1,4})\s+(.+)$/gm : /^(#{1,4})\s+(.+)$/gm
  let match: RegExpExecArray | null
  while ((match = regex.exec(content)) !== null) {
    const level = match[1].length
    const text = cleanHeadingText(match[2], format)
    const id = text
      .toLowerCase()
      .replace(/[^\w\s-]/g, '')
      .replace(/\s+/g, '-')
    headings.push({ level, text, id })
  }
  return headings
}

function cleanHeadingText(text: string, format: NoteFileFormat): string {
  const trimmed = text.trim()
  if (format === 'markdown') return trimmed
  return trimmed
    .replace(/^(TODO|DONE)\s+/, '')
    .replace(/\s+:[\w@#%:]+:\s*$/, '')
    .trim()
}

// DocsToc renders a table of contents extracted from note headings.
// Clicking a heading scrolls the Lexical editor to the corresponding element.
function DocsToc({ content, format }: DocsTocProps) {
  const headings = useMemo(
    () => parseHeadings(content, format),
    [content, format],
  )
  const containerRef = useRef<HTMLDivElement>(null)

  const handleClick = useCallback((heading: TocHeading) => {
    // Lexical renders headings as <h1>-<h4> elements. Find the matching
    // heading in the editor content area by walking the DOM.
    const container = containerRef.current?.closest('.bg-background-primary')
    if (!container) return

    const editorArea = container.querySelector('[contenteditable]')
    if (!editorArea) return

    const tag = `h${heading.level}`
    const elements = editorArea.querySelectorAll(tag)
    for (const el of elements) {
      if (el.textContent?.trim() === heading.text) {
        el.scrollIntoView({ behavior: 'smooth', block: 'start' })
        return
      }
    }
  }, [])

  if (headings.length === 0) {
    return null
  }

  // Find the minimum heading level to normalize indentation.
  const minLevel = Math.min(...headings.map((h) => h.level))

  return (
    <div ref={containerRef} className="flex h-full flex-col overflow-y-auto">
      <div className="text-foreground-alt border-border flex items-center gap-1.5 border-b px-3 py-2 text-xs font-medium tracking-wide uppercase">
        <LuList className="size-3" />
        On this page
      </div>
      <nav className="flex-1 overflow-y-auto py-1">
        {headings.map((heading) => {
          const indent = (heading.level - minLevel) * 12
          return (
            <button
              key={heading.id}
              type="button"
              className={cn(
                'text-foreground-alt hover:text-foreground block w-full truncate py-1 pr-3 text-left text-xs',
                'hover:bg-list-hover-background',
              )}
              style={{ paddingLeft: 12 + indent }}
              onClick={() => handleClick(heading)}
            >
              {heading.text}
            </button>
          )
        })}
      </nav>
    </div>
  )
}

export default DocsToc
