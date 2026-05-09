import { useState, useRef, useCallback, type KeyboardEvent } from 'react'
import { LuSearch } from 'react-icons/lu'
import { cn } from '../style/utils.js'

interface SearchBoxProps {
  placeholder?: string
  className?: string
  focusOnMount?: boolean
  onSearch?: (query: string) => void
  onBlur?: () => void
}

// SearchBox is an expandable search input with icon.
export function SearchBox({
  placeholder = 'Search',
  className,
  focusOnMount = false,
  onSearch,
  onBlur,
}: SearchBoxProps) {
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

  const handleSearchBlur = useCallback(() => {
    setFocused(false)
    if (query.trim() && onSearch) {
      onSearch(query)
    }
    onBlur?.()
  }, [query, onSearch, onBlur])

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
      className={cn(
        'bg-file-search-box relative flex items-center gap-1 rounded px-2 py-0.5 transition-all duration-200',
        focused ? 'w-48' : 'w-7',
        className,
      )}
    >
      <LuSearch className="text-foreground-alt size-3 flex-shrink-0" />
      {focused && (
        <input
          ref={setInputRef}
          type="text"
          placeholder={placeholder}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          className="text-foreground w-full bg-transparent text-xs outline-none"
          onBlur={handleSearchBlur}
        />
      )}
      {!focused && (
        <button
          className="absolute inset-0"
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
