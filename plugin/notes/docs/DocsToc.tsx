import { useCallback, useMemo, useRef } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { LuList } from 'react-icons/lu'

interface TocHeading {
  level: number
  text: string
  id: string
}

interface DocsTocProps {
  markdown: string
}

// parseHeadings extracts h1-h4 headings from markdown content.
function parseHeadings(markdown: string): TocHeading[] {
  const headings: TocHeading[] = []
  const regex = /^(#{1,4})\s+(.+)$/gm
  let match: RegExpExecArray | null
  while ((match = regex.exec(markdown)) !== null) {
    const level = match[1].length
    const text = match[2].trim()
    const id = text
      .toLowerCase()
      .replace(/[^\w\s-]/g, '')
      .replace(/\s+/g, '-')
    headings.push({ level, text, id })
  }
  return headings
}

// DocsToc renders a table of contents extracted from markdown headings.
// Clicking a heading scrolls the Lexical editor to the corresponding element.
function DocsToc({ markdown }: DocsTocProps) {
  const headings = useMemo(() => parseHeadings(markdown), [markdown])
  const containerRef = useRef<HTMLDivElement>(null)

  const handleClick = useCallback((heading: TocHeading) => {
    // Lexical renders headings as <h1>-<h4> elements. Find the matching
    // heading in the editor content area by walking the DOM.
    const container = containerRef.current?.closest(
      '.bg-background-primary',
    )
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
      <div className="text-foreground-alt flex items-center gap-1.5 border-b border-border px-3 py-2 text-xs font-medium uppercase tracking-wide">
        <LuList className="h-3 w-3" />
        On this page
      </div>
      <nav className="flex-1 overflow-y-auto py-1">
        {headings.map((heading, index) => {
          const indent = (heading.level - minLevel) * 12
          return (
            <button
              key={`${heading.id}-${index}`}
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
