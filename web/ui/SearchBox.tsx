import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { useState, useRef, useCallback, type KeyboardEvent } from 'react'
import { LuSearch } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '.25rem',
    backgroundColor: themeColors['--color-file-search-box'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
  },
  rule2: { width: `calc(${themeLayout['--spacing']} * 48)` },
  rule3: { width: `calc(${themeLayout['--spacing']} * 7)` },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    width: '100%',
    backgroundColor: '#0000',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
  rule6: { position: 'absolute', inset: 0 },
})

interface SearchBoxProps {
  className?: never
  placeholder?: string
  sx?: ComponentStyles
  focusOnMount?: boolean
  onSearch?: (query: string) => void
  onBlur?: () => void
}

// SearchBox is an expandable search input with icon.
export function SearchBox({
  placeholder = 'Search',
  sx,
  focusOnMount = false,
  onSearch,
  onBlur,
}: Omit<SearchBoxProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  // Retain pending focus requests until the search input mounts.
  const initialFocusRef = useRef(focusOnMount)
  const [focused, setFocused] = useState(() => initialFocusRef.current)
  const [query, setQuery] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const focusRequestedRef = useRef(initialFocusRef.current)
  const focusInputSoon = useCallback(() => {
    focusRequestedRef.current = true
    queueMicrotask(() => inputRef.current?.focus())
  }, [])
  const setInputRef = useCallback((el: HTMLInputElement | null) => {
    inputRef.current = el
    if (el && focusRequestedRef.current) {
      focusRequestedRef.current = false
      el.focus()
    }
  }, [])

  // Commit a nonempty search before notifying the caller that focus left.
  const handleSearchBlur = useCallback(() => {
    setFocused(false)
    if (query.trim() && onSearch) {
      onSearch(query)
    }
    onBlur?.()
  }, [query, onSearch, onBlur])

  // Commit the current query when Enter is pressed.
  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter' && query.trim() && onSearch) {
        onSearch(query)
      }
    },
    [query, onSearch],
  )

  return (
    <div
      sx={[
        stylexStyles.rule1,
        focused ? stylexStyles.rule2 : stylexStyles.rule3,
        sx,
      ]}
    >
      <LuSearch {...stylex.props(stylexStyles.rule4)} />
      {focused && (
        <input
          ref={setInputRef}
          type="text"
          placeholder={placeholder}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          sx={stylexStyles.rule5}
          onBlur={handleSearchBlur}
        />
      )}
      {!focused && (
        <button
          type="button"
          sx={stylexStyles.rule6}
          onClick={() => {
            setFocused(true)
            focusInputSoon()
          }}
          aria-label="Open search"
        />
      )}
    </div>
  )
}
