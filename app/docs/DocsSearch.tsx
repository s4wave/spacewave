import { useState, useCallback, useMemo, useRef } from 'react'
import { LuSearch } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import type { DocPage } from './types.js'

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
    return docs.filter(
      (d) =>
        d.title.toLowerCase().includes(q) ||
        d.body.toLowerCase().includes(q) ||
        d.summary.toLowerCase().includes(q),
    )
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

  const handleFocus = useCallback(() => setFocused(true), [])
  const handleBlur = useCallback(() => {
    // Delay to allow click on result.
    setTimeout(() => setFocused(false), 150)
  }, [])

  const handleChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setQuery(e.target.value)
  }, [])

  const showDropdown = focused && query.trim().length > 0

  return (
    <div className="relative">
      <div className="relative">
        <LuSearch className="text-foreground-alt/40 absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={handleChange}
          onFocus={handleFocus}
          onBlur={handleBlur}
          placeholder="Search docs..."
          className={cn(
            'border-foreground/10 bg-background-card/50 text-foreground placeholder:text-foreground-alt/40 w-full rounded-md border py-2 pr-3 pl-9 text-sm transition-colors outline-none',
            'focus:border-foreground/20',
          )}
        />
      </div>

      {showDropdown && (
        <div className="border-foreground/15 bg-background-card absolute top-full right-0 left-0 z-50 mt-1 max-h-64 overflow-y-auto rounded-md border shadow-lg">
          {results.length === 0 && (
            <div className="text-foreground-alt/50 px-4 py-3 text-sm">
              No results found.
            </div>
          )}
          {results.map((doc) => (
            <button
              key={doc.url}
              onMouseDown={() => handleSelect(doc)}
              className="hover:bg-foreground/5 w-full cursor-pointer px-4 py-2.5 text-left transition-colors"
            >
              <div className="text-foreground text-sm font-medium">
                {doc.title}
              </div>
              <div className="text-foreground-alt/50 mt-0.5 text-xs">
                {doc.summary}
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
