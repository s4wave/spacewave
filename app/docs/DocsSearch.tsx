import { useState, useCallback, useMemo, useRef } from 'react'
import { LuSearch } from '@s4wave/web/images/icons.js'

import type { DocPage } from './types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { position: 'relative' },
  rule2: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 9)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
  },
  rule3: {
    position: 'absolute',
    top: '100%',
    right: 0,
    left: 0,
    zIndex: 50,
    marginTop: themeLayout['--spacing'],
    maxHeight: `calc(${themeLayout['--spacing']} * 64)`,
    overflowY: 'auto',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    backgroundColor: themeColors['--color-background-card'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule4: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule5: {
    width: '100%',
    cursor: 'pointer',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule7: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    position: 'absolute',
    top: '50%',
    left: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
})

// DocsSearchProps defines the props for DocsSearch.
interface DocsSearchProps {
  docs: DocPage[]
  onSelect: (doc: DocPage) => void
}

// DocsSearch renders a client-side search input with dropdown results.
export function DocsSearch({ docs, onSelect }: DocsSearchProps) {
  const [query, setQuery] = useState('')
  const [focused, setFocused] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const results = useMemo(() => {
    if (!query.trim()) return []
    const q = query.toLowerCase()
    return docs
      .flatMap((doc, index) => {
        const title = doc.title.toLowerCase()
        const titleMatch = title.includes(q)
        const contentMatch =
          doc.body.toLowerCase().includes(q) ||
          doc.summary.toLowerCase().includes(q)
        if (!titleMatch && !contentMatch) return []

        return [
          {
            doc,
            index,
            score: title === q ? 3 : titleMatch ? 2 : 1,
          },
        ]
      })
      .sort((a, b) => b.score - a.score || a.index - b.index)
      .map(({ doc }) => doc)
  }, [query, docs])

  const handleSelect = useCallback(
    (doc: DocPage) => {
      setQuery('')
      setFocused(false)
      inputRef.current?.blur()
      onSelect(doc)
    },
    [onSelect],
  )

  const handleSearchFocus = useCallback(() => setFocused(true), [])
  const handleSearchBlur = useCallback(() => {
    // Delay to allow click on result.
    setTimeout(() => setFocused(false), 150)
  }, [])

  const handleSearchChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setQuery(e.target.value)
    },
    [],
  )

  const showDropdown = focused && query.trim().length > 0

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule1}>
        <LuSearch sx={stylexStyles.rule1} />
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={handleSearchChange}
          onFocus={handleSearchFocus}
          onBlur={handleSearchBlur}
          aria-label="Search documentation"
          placeholder="Search docs…"
          sx={[stylexStyles2.rule2]}
        />
      </div>

      {showDropdown && (
        <div sx={stylexStyles2.rule3}>
          {results.length === 0 && (
            <div sx={stylexStyles2.rule4}>No results found.</div>
          )}
          {results.map((doc) => (
            <button
              key={doc.url}
              type="button"
              onClick={() => handleSelect(doc)}
              sx={stylexStyles2.rule5}
            >
              <div sx={stylexStyles2.rule6}>{doc.title}</div>
              <div sx={stylexStyles2.rule7}>{doc.summary}</div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
