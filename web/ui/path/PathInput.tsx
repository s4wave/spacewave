import {
  useState,
  useRef,
  useEffect,
  useMemo,
  useCallback,
  type KeyboardEvent,
} from 'react'
import { LuChevronRight, LuHouse } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'

interface PathInputProps {
  path: string
  onPathChange?: (path: string) => void
  onNavigate?: (path: string) => void
  className?: string
}

// PathInput renders an editable path breadcrumb component.
export function PathInput({
  path,
  onPathChange,
  onNavigate,
  className,
}: PathInputProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [editValue, setEditValue] = useState(path)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    setEditValue(path)
  }, [path])

  useEffect(() => {
    if (isEditing && inputRef.current) {
      inputRef.current.focus()
      inputRef.current.select()
    }
  }, [isEditing])

  const pathSegments = useMemo(() => path.split('/').filter(Boolean), [path])

  const handleBreadcrumbClick = useCallback(
    (index: number) => {
      const newPath = '/' + pathSegments.slice(0, index + 1).join('/')
      onNavigate?.(newPath)
    },
    [pathSegments, onNavigate],
  )

  const handleRootClick = useCallback(() => {
    onNavigate?.('/')
  }, [onNavigate])

  const handleContainerClick = useCallback(() => {
    setIsEditing(true)
  }, [])

  const handleInputBlur = useCallback(() => {
    setIsEditing(false)
    if (editValue !== path) {
      onPathChange?.(editValue)
    }
  }, [editValue, path, onPathChange])

  const handleInputKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        setIsEditing(false)
        if (editValue !== path) {
          onPathChange?.(editValue)
        }
      }
      if (e.key === 'Escape') {
        setIsEditing(false)
        setEditValue(path)
      }
    },
    [editValue, path, onPathChange],
  )

  if (isEditing) {
    return (
      <div
        className={cn(
          'bg-file-path-bar flex h-5 flex-1 items-center rounded px-2',
          className,
        )}
      >
        <input
          ref={inputRef}
          type="text"
          value={editValue}
          onChange={(e) => setEditValue(e.target.value)}
          onBlur={handleInputBlur}
          onKeyDown={handleInputKeyDown}
          className="text-foreground w-full bg-transparent font-mono text-xs outline-none"
          spellCheck={false}
          autoComplete="off"
        />
      </div>
    )
  }

  return (
    <div
      onClick={handleContainerClick}
      className={cn(
        'bg-file-path-bar hover:bg-file-path-bar-hover text-foreground flex h-5 flex-1 cursor-text items-center gap-0.5 overflow-hidden rounded px-2 text-xs transition-colors select-none',
        className,
      )}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          setIsEditing(true)
        }
      }}
      aria-label="File path"
    >
      <button
        onClick={(e) => {
          e.stopPropagation()
          handleRootClick()
        }}
        className={cn(
          'hover:bg-pulldown-hover flex items-center rounded px-1 transition-colors',
          pathSegments.length === 0 && 'text-text-highlight',
        )}
        aria-label="Navigate to root"
      >
        <LuHouse className="h-3.5 w-3.5" />
      </button>

      {pathSegments.map((segment, index) => (
        <div key={index} className="flex items-center">
          <LuChevronRight className="text-foreground-alt h-3 w-3" />
          <button
            onClick={(e) => {
              e.stopPropagation()
              handleBreadcrumbClick(index)
            }}
            className={cn(
              'hover:bg-pulldown-hover rounded px-1 whitespace-nowrap transition-colors',
              index === pathSegments.length - 1 && 'text-text-highlight',
            )}
            aria-label={`Navigate to ${segment}`}
          >
            {segment}
          </button>
        </div>
      ))}
    </div>
  )
}
