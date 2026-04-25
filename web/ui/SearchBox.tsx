import {
  useState,
  useEffect,
  useRef,
  useCallback,
  type KeyboardEvent,
} from 'react'
import { LuSearch } from 'react-icons/lu'
import { cn } from '../style/utils.js'

interface SearchBoxProps {
  placeholder?: string
  className?: string
  autoFocus?: boolean
  onSearch?: (query: string) => void
  onBlur?: () => void
}

// SearchBox is an expandable search input with icon.
export function SearchBox({
  placeholder = 'Search',
  className,
  autoFocus = false,
  onSearch,
  onBlur,
}: SearchBoxProps) {
  const [focused, setFocused] = useState(autoFocus)
  const [query, setQuery] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (autoFocus && inputRef.current) {
      inputRef.current.focus()
    }
  }, [autoFocus])

  const handleBlur = useCallback(() => {
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
      <LuSearch className="text-foreground-alt h-3 w-3 flex-shrink-0" />
      {focused && (
        <input
          ref={inputRef}
          type="text"
          placeholder={placeholder}
          autoFocus
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          className="text-foreground w-full bg-transparent text-xs outline-none"
          onBlur={handleBlur}
        />
      )}
      {!focused && (
        <button
          className="absolute inset-0"
          onClick={() => setFocused(true)}
          aria-label="Open search"
        />
      )}
    </div>
  )
}
