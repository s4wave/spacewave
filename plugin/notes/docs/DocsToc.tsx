import { useCallback, useMemo, useRef } from 'react'

import { LuList } from '@s4wave/web/images/icons.js'

import type { NoteFileFormat } from '../note-files.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    overflowY: 'auto',
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    borderColor: themeColors['--color-border'],
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule3: { flex: 1, paddingBlock: themeLayout['--spacing'], overflowY: 'auto', },
  rule4: {
    overflow: 'hidden',
    paddingBlock: themeLayout['--spacing'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    paddingLeft: 'var(--docs-tree-padding-left)',
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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
    <div ref={containerRef} sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <LuList sx={stylexStyles.rule1} />
        On this page
      </div>
      <nav sx={stylexStyles2.rule3}>
        {headings.map((heading) => {
          const indent = (heading.level - minLevel) * 12
          return (
            <button
              key={heading.id}
              type="button"
              sx={[stylexStyles2.rule4]}
              style={{ '--docs-tree-padding-left': `${12 + indent}px` }}
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
