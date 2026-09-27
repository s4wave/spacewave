/* eslint-disable react-doctor/rerender-state-only-in-handlers */
import {
  useState,
  useRef,
  useEffect,
  useMemo,
  useCallback,
  type DragEvent,
  type KeyboardEvent,
} from 'react'
import { LuChevronRight, LuHouse } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'

interface PathInputProps {
  path: string
  onPathChange?: (path: string) => void
  onNavigate?: (path: string) => void
  onPathTargetDragOver?: (
    path: string,
    event: DragEvent<HTMLElement>,
  ) => boolean
  onPathTargetDrop?: (path: string, event: DragEvent<HTMLElement>) => void
  className?: string
  mobileTouchTargets?: boolean
}

/** PathInput renders an editable path with breadcrumb navigation. */
export function PathInput({
  path,
  onPathChange,
  onNavigate,
  onPathTargetDragOver,
  onPathTargetDrop,
  className,
  mobileTouchTargets = false,
}: PathInputProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [editState, setEditState] = useState({ path, value: path })
  const inputRef = useRef<HTMLInputElement>(null)
  const editValue = editState.path === path ? editState.value : path
  const setEditValue = useCallback(
    (value: string) => setEditState({ path, value }),
    [path],
  )

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
        setEditState({ path, value: path })
      }
    },
    [editValue, path, onPathChange],
  )

  if (isEditing) {
    return (
      <div
        className={cn(
          'bg-file-path-bar flex h-5 flex-1 items-center rounded px-2',
          mobileTouchTargets && '[@media(pointer:coarse)]:min-h-11',
          className,
        )}
      >
        <input
          ref={inputRef}
          type="text"
          aria-label="File path"
          value={editValue}
          onChange={(e) => setEditValue(e.target.value)}
          onBlur={handleInputBlur}
          onKeyDown={handleInputKeyDown}
          className={cn(
            'text-foreground w-full bg-transparent font-mono text-xs outline-none',
            mobileTouchTargets && '[@media(pointer:coarse)]:text-base',
          )}
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
        mobileTouchTargets &&
          'min-w-0 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:gap-0 [@media(pointer:coarse)]:px-0 [@media(pointer:coarse)]:text-sm',
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
        type="button"
        onClick={(e) => {
          e.stopPropagation()
          handleRootClick()
        }}
        onDragOver={(e) => onPathTargetDragOver?.('/', e)}
        onDrop={(e) => onPathTargetDrop?.('/', e)}
        className={cn(
          'hover:bg-pulldown-hover flex items-center rounded px-1 transition-colors',
          mobileTouchTargets &&
            '[@media(pointer:coarse)]:size-11 [@media(pointer:coarse)]:shrink-0 [@media(pointer:coarse)]:justify-center [@media(pointer:coarse)]:px-0',
          pathSegments.length === 0 && 'text-text-highlight',
        )}
        aria-label="Navigate to root"
      >
        <LuHouse className="size-3.5" />
      </button>

      {pathSegments.map((segment, index) => (
        <div
          key={pathSegments.slice(0, index + 1).join('/')}
          className={cn(
            'flex items-center',
            mobileTouchTargets &&
              (index === pathSegments.length - 1
                ? 'max-sm:min-w-0 max-sm:flex-1'
                : 'max-sm:hidden'),
          )}
        >
          <LuChevronRight className="text-foreground-alt size-3" />
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              handleBreadcrumbClick(index)
            }}
            onDragOver={(e) =>
              onPathTargetDragOver?.(
                '/' + pathSegments.slice(0, index + 1).join('/'),
                e,
              )
            }
            onDrop={(e) =>
              onPathTargetDrop?.(
                '/' + pathSegments.slice(0, index + 1).join('/'),
                e,
              )
            }
            className={cn(
              'hover:bg-pulldown-hover rounded px-1 whitespace-nowrap transition-colors',
              mobileTouchTargets &&
                '[@media(pointer:coarse)]:min-h-11 max-sm:min-w-0 max-sm:flex-1 max-sm:truncate max-sm:px-2 max-sm:text-left',
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
