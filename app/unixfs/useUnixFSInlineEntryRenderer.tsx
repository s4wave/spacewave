import { useMemo } from 'react'
import { LuCheck, LuX } from 'react-icons/lu'

import type { RenderEntryCallback } from '@s4wave/web/editors/file-browser/FileListEntry.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'

interface UnixFSInlineEntryRendererOptions {
  newFolderName: string | null
  newFileName: string | null
  renamingEntry: FileEntry | null
  renameRef: { current: string }
  onConfirmRename: () => Promise<void>
  onCancelRename: () => void
  onNewFolderNameChange: (name: string) => void
  onNewFileNameChange: (name: string) => void
  onNewFolderConfirm: (name: string) => Promise<void>
  onNewFolderCancel: () => void
  onNewFileConfirm: (name: string) => Promise<void>
  onNewFileCancel: () => void
}

export function useUnixFSInlineEntryRenderer({
  newFolderName,
  newFileName,
  renamingEntry,
  renameRef,
  onConfirmRename,
  onCancelRename,
  onNewFolderNameChange,
  onNewFileNameChange,
  onNewFolderConfirm,
  onNewFolderCancel,
  onNewFileConfirm,
  onNewFileCancel,
}: UnixFSInlineEntryRendererOptions): RenderEntryCallback | undefined {
  return useMemo(() => {
    if (newFolderName === null && newFileName === null && !renamingEntry) {
      return undefined
    }
    return ({ entry, defaultNode }) => {
      const isNewFolder =
        entry.id === '__new-folder__' && newFolderName !== null
      const isNewFile = entry.id === '__new-file__' && newFileName !== null
      const isRenaming = !!renamingEntry && entry.id === renamingEntry.id
      const isNewItem = isNewFolder || isNewFile

      if (!isNewItem && !isRenaming) return defaultNode

      if (isRenaming) {
        return (
          <div
            role="presentation"
            className="rename-actions flex min-w-[120px] flex-1 items-center gap-0.5 overflow-hidden"
            onClick={(event) => event.stopPropagation()}
            onMouseDown={(event) => event.stopPropagation()}
          >
            <input
              ref={(element) => {
                if (!element) return
                element.focus()
                const name = renameRef.current
                const lastDot = name.lastIndexOf('.')
                element.setSelectionRange(
                  0,
                  lastDot > 0 ? lastDot : name.length,
                )
              }}
              className="bg-background text-foreground border-brand min-w-0 flex-1 rounded border px-1.5 py-0.5 text-xs outline-none"
              defaultValue={renameRef.current}
              onChange={(event) => {
                renameRef.current = event.target.value
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  void onConfirmRename()
                }
                if (event.key === 'Escape') {
                  event.preventDefault()
                  onCancelRename()
                }
                event.stopPropagation()
              }}
              onBlur={(event) => {
                const related = event.relatedTarget as HTMLElement | null
                if (related?.closest('.rename-actions')) return
                onCancelRename()
              }}
            />
            <button
              tabIndex={0}
              aria-label="Confirm rename"
              className="text-brand hover:text-brand-highlight shrink-0 p-0.5"
              onClick={(event) => {
                event.preventDefault()
                event.stopPropagation()
                void onConfirmRename()
              }}
            >
              <LuCheck className="size-3" />
            </button>
            <button
              tabIndex={0}
              aria-label="Cancel rename"
              className="text-foreground-alt hover:text-foreground shrink-0 p-0.5"
              onClick={(event) => {
                event.preventDefault()
                event.stopPropagation()
                onCancelRename()
              }}
            >
              <LuX className="size-3" />
            </button>
          </div>
        )
      }

      const value = isNewFolder ? newFolderName : newFileName!
      const placeholder = isNewFolder ? 'Folder name' : 'File name'
      const handleConfirm = isNewFolder ? onNewFolderConfirm : onNewFileConfirm
      const handleCancel = isNewFolder ? onNewFolderCancel : onNewFileCancel
      const handleNameChange = isNewFolder
        ? onNewFolderNameChange
        : onNewFileNameChange

      return (
        <div
          role="presentation"
          className="flex min-w-[120px] flex-1 items-center gap-2 overflow-hidden"
          onClick={(event) => event.stopPropagation()}
          onMouseDown={(event) => event.stopPropagation()}
        >
          <input
            ref={(element) => element?.focus()}
            className="bg-background text-foreground border-brand min-w-0 flex-1 rounded border px-1 py-0 text-xs outline-none"
            value={value}
            onChange={(event) => handleNameChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                void handleConfirm(event.currentTarget.value)
              }
              if (event.key === 'Escape') {
                event.preventDefault()
                handleCancel()
              }
              event.stopPropagation()
            }}
            onBlur={(event) => {
              if (event.currentTarget.value.trim()) {
                void handleConfirm(event.currentTarget.value)
              } else {
                handleCancel()
              }
            }}
            placeholder={placeholder}
          />
        </div>
      )
    }
  }, [
    newFolderName,
    newFileName,
    renamingEntry,
    renameRef,
    onConfirmRename,
    onCancelRename,
    onNewFolderNameChange,
    onNewFileNameChange,
    onNewFolderConfirm,
    onNewFolderCancel,
    onNewFileConfirm,
    onNewFileCancel,
  ])
}
