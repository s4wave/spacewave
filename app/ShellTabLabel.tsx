import { useState, useCallback, useRef, useEffect, KeyboardEvent } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { getTabDisplayName, type ShellTab } from '@s4wave/app/shell-tab.js'

import { useShellTabs } from './ShellTabContext.js'

// ShellTabLabelProps are the props for ShellTabLabel.
interface ShellTabLabelProps {
  tab: ShellTab
}

// ShellTabLabel renders a tab label that supports inline rename.
// Double-click the label or use "Rename Tab" from the context menu to enter edit mode.
// Press Enter or blur to save. Press Escape to cancel.
// Clearing the input reverts to the auto-derived default name.
export function ShellTabLabel({ tab }: ShellTabLabelProps) {
  const { updateTabName, renamingTabId, stopRenaming } = useShellTabs()
  const [editing, setEditing] = useState(false)
  const [editValue, setEditValue] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const displayName = getTabDisplayName(tab)

  // Enter edit mode when this tab is targeted for renaming via context menu.
  // Defer stopRenaming so editing state commits and input mounts before the
  // renamingTabId is cleared.
  useEffect(() => {
    if (renamingTabId === tab.id) {
      setEditValue(displayName)
      setEditing(true)
      queueMicrotask(stopRenaming)
    }
  }, [renamingTabId, tab.id, displayName, stopRenaming])

  // Focus input when entering edit mode
  useEffect(() => {
    if (editing && inputRef.current) {
      inputRef.current.focus()
      inputRef.current.select()
    }
  }, [editing])

  const handleDoubleClick = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      setEditValue(displayName)
      setEditing(true)
    },
    [displayName],
  )

  const handleSave = useCallback(() => {
    const trimmed = editValue.trim()
    // Empty string clears custom name, reverting to default
    updateTabName(tab.id, trimmed)
    setEditing(false)
  }, [editValue, tab.id, updateTabName])

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      e.stopPropagation()
      if (e.key === 'Enter') {
        e.preventDefault()
        handleSave()
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        setEditing(false)
      }
    },
    [handleSave],
  )

  if (editing) {
    return (
      <input
        ref={inputRef}
        className={cn(
          'bg-background-secondary text-foreground rounded-menu-button',
          'border-none outline-none',
          'text-[0.6875rem] leading-5 font-medium tracking-[-0.01em]',
          'w-full max-w-64 min-w-12 px-1 py-0',
        )}
        value={editValue}
        onChange={(e) => setEditValue(e.target.value)}
        onBlur={handleSave}
        onKeyDown={handleKeyDown}
        onMouseDown={(e) => e.stopPropagation()}
        onClick={(e) => e.stopPropagation()}
      />
    )
  }

  return (
    <span className="truncate" onDoubleClick={handleDoubleClick}>
      {displayName}
    </span>
  )
}
