import { useCallback, useState, type KeyboardEvent } from 'react'

import { getTabDisplayName, type ShellTab } from '@s4wave/app/shell-tab.js'
import { cn } from '@s4wave/web/style/utils.js'

import { useShellTabs } from './ShellTabContext.js'

interface ShellTabLabelProps {
  tab: ShellTab
}

interface ShellTabNameEditorProps {
  displayName: string
  onCancel: () => void
  onSave: (name: string) => void
}

function ShellTabNameEditor({
  displayName,
  onCancel,
  onSave,
}: ShellTabNameEditorProps) {
  const [value, setValue] = useState(displayName)
  const inputRef = useCallback((input: HTMLInputElement | null) => {
    input?.focus()
    input?.select()
  }, [])
  const save = useCallback(() => onSave(value.trim()), [onSave, value])
  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>) => {
      event.stopPropagation()
      if (event.key === 'Enter') {
        event.preventDefault()
        save()
      } else if (event.key === 'Escape') {
        event.preventDefault()
        onCancel()
      }
    },
    [onCancel, save],
  )

  return (
    <input
      ref={inputRef}
      className={cn(
        'bg-background-secondary text-foreground rounded-menu-button',
        'border-none outline-none',
        'text-[0.6875rem] leading-5 font-medium tracking-[-0.01em]',
        'w-full max-w-64 min-w-12 px-1 py-0',
      )}
      value={value}
      onChange={(event) => setValue(event.target.value)}
      onBlur={save}
      onKeyDown={handleKeyDown}
      onMouseDown={(event) => event.stopPropagation()}
      onClick={(event) => event.stopPropagation()}
    />
  )
}

// ShellTabLabel renders a tab label that supports inline rename.
export function ShellTabLabel({ tab }: ShellTabLabelProps) {
  const { updateTabName, renamingTabId, stopRenaming } = useShellTabs()
  const [localRename, setLocalRename] = useState(false)
  const displayName = getTabDisplayName(tab)
  const renaming = localRename || renamingTabId === tab.id

  const finishRename = useCallback(() => {
    setLocalRename(false)
    stopRenaming(tab.id)
  }, [stopRenaming, tab.id])
  const handleSave = useCallback(
    (name: string) => {
      updateTabName(tab.id, name)
      finishRename()
    },
    [finishRename, tab.id, updateTabName],
  )
  const handleDoubleClick = useCallback((event: React.MouseEvent) => {
    event.preventDefault()
    event.stopPropagation()
    setLocalRename(true)
  }, [])

  if (renaming) {
    return (
      <ShellTabNameEditor
        displayName={displayName}
        onCancel={finishRename}
        onSave={handleSave}
      />
    )
  }

  return (
    <span className="truncate" onDoubleClick={handleDoubleClick}>
      {displayName}
    </span>
  )
}
