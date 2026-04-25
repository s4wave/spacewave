import { useCallback, useMemo } from 'react'
import {
  LuFolderOpen,
  LuDownload,
  LuPencil,
  LuFolderInput,
  LuTrash2,
  LuFolderPlus,
  LuUpload,
} from 'react-icons/lu'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { DropdownMenuGhostAnchor } from '@s4wave/web/ui/DropdownMenuGhostAnchor.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import type { UnixFSMoveItem } from './move.js'

export interface ContextMenuState {
  position: { x: number; y: number }
  entry: FileEntry | null
  actionEntries: FileEntry[]
  moveItems: UnixFSMoveItem[]
}

export interface UnixFSContextMenuProps {
  state: ContextMenuState | null
  onClose: () => void
  onOpen?: (entries: FileEntry[]) => void
  onDownload?: (entries: FileEntry[]) => void
  onMove?: (moveItems: UnixFSMoveItem[]) => void
  onRename?: (entry: FileEntry) => void
  onDelete?: (entries: FileEntry[]) => void
  onNewFolder?: () => void
  onUploadFiles?: () => void
}

// UnixFSContextMenu renders a context menu at the given position using the
// ghost-anchor DropdownMenu pattern (see ShellFlexLayout.tsx). When
// state.entry is non-null, shows item actions. When null, shows background
// actions (new folder, upload).
export function UnixFSContextMenu({
  state,
  onClose,
  onOpen,
  onDownload,
  onMove,
  onRename,
  onDelete,
  onNewFolder,
  onUploadFiles,
}: UnixFSContextMenuProps) {
  const entry = state?.entry ?? null
  const actionEntries = useMemo(
    () => state?.actionEntries ?? [],
    [state?.actionEntries],
  )

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) onClose()
    },
    [onClose],
  )

  const handleOpen = useCallback(() => {
    if (!entry) return
    onOpen?.([entry])
  }, [entry, onOpen])

  const handleDownload = useCallback(() => {
    if (actionEntries.length === 0) return
    onDownload?.(actionEntries)
  }, [actionEntries, onDownload])

  const handleNewFolder = useCallback(() => {
    onNewFolder?.()
  }, [onNewFolder])

  const handleMove = useCallback(() => {
    if (state?.moveItems.length === 0) return
    onMove?.(state?.moveItems ?? [])
  }, [onMove, state?.moveItems])

  const handleRename = useCallback(() => {
    if (!entry) return
    onRename?.(entry)
  }, [entry, onRename])

  const handleDelete = useCallback(() => {
    if (actionEntries.length === 0) return
    onDelete?.(actionEntries)
  }, [actionEntries, onDelete])

  return (
    <DropdownMenu open={state !== null} onOpenChange={handleOpenChange}>
      <DropdownMenuTrigger asChild>
        <DropdownMenuGhostAnchor
          x={state?.position.x ?? 0}
          y={state?.position.y ?? 0}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" side="bottom">
        {state?.entry ?
          <>
            <DropdownMenuItem onClick={handleOpen}>
              <LuFolderOpen className="h-3.5 w-3.5" />
              Open
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleNewFolder} disabled={!onNewFolder}>
              <LuFolderPlus className="h-3.5 w-3.5" />
              New folder
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={handleDownload}
              disabled={!onDownload || actionEntries.length === 0}
            >
              <LuDownload className="h-3.5 w-3.5" />
              {actionEntries.length > 1 ? 'Download selected' : 'Download'}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={handleRename} disabled={!onRename}>
              <LuPencil className="h-3.5 w-3.5" />
              Rename
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleMove} disabled={!onMove}>
              <LuFolderInput className="h-3.5 w-3.5" />
              Move...
            </DropdownMenuItem>
            <DropdownMenuItem
              variant="destructive"
              onClick={handleDelete}
              disabled={!onDelete}
            >
              <LuTrash2 className="h-3.5 w-3.5" />
              Delete
              <DropdownMenuShortcut>Del</DropdownMenuShortcut>
            </DropdownMenuItem>
          </>
        : <>
            <DropdownMenuItem onClick={handleNewFolder} disabled={!onNewFolder}>
              <LuFolderPlus className="h-3.5 w-3.5" />
              New folder
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onUploadFiles} disabled={!onUploadFiles}>
              <LuUpload className="h-3.5 w-3.5" />
              Upload files
            </DropdownMenuItem>
          </>
        }
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
