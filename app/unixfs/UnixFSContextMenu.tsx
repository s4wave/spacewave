import { useCallback } from 'react'
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

/** UnixFSContextMenu shows the actions available at the clicked file or folder. */
export function UnixFSContextMenu(props: UnixFSContextMenuProps) {
  // Anchor the context menu at the last pointer position.
  const { state, onClose } = props
  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) onClose()
    },
    [onClose],
  )

  return (
    <DropdownMenu open={state !== null} onOpenChange={handleOpenChange}>
      <DropdownMenuTrigger asChild>
        <DropdownMenuGhostAnchor
          x={state?.position.x ?? 0}
          y={state?.position.y ?? 0}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" side="bottom">
        {state?.entry ? (
          <EntryMenuItems {...props} state={state} entry={state.entry} />
        ) : (
          <FolderMenuItems
            onNewFolder={props.onNewFolder}
            onUploadFiles={props.onUploadFiles}
          />
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** FolderMenuItems offers creation actions where the folder supports them. */
function FolderMenuItems({
  onNewFolder,
  onUploadFiles,
}: Pick<UnixFSContextMenuProps, 'onNewFolder' | 'onUploadFiles'>) {
  return (
    <>
      {onNewFolder && (
        <DropdownMenuItem onClick={onNewFolder}>
          <LuFolderPlus className="size-3.5" />
          New folder
        </DropdownMenuItem>
      )}
      {onUploadFiles && (
        <DropdownMenuItem onClick={onUploadFiles}>
          <LuUpload className="size-3.5" />
          Upload files
        </DropdownMenuItem>
      )}
    </>
  )
}

/** EntryMenuItems acts on the clicked entry or the selected set. */
function EntryMenuItems({
  state,
  entry,
  onOpen,
  onDownload,
  onMove,
  onRename,
  onDelete,
  onNewFolder,
}: UnixFSContextMenuProps & { state: ContextMenuState; entry: FileEntry }) {
  const { actionEntries, moveItems } = state
  return (
    <>
      <DropdownMenuItem onClick={() => onOpen?.([entry])}>
        <LuFolderOpen className="size-3.5" />
        Open
      </DropdownMenuItem>
      <FolderMenuItems onNewFolder={onNewFolder} />
      {onDownload && actionEntries.length > 0 && (
        <DropdownMenuItem onClick={() => onDownload(actionEntries)}>
          <LuDownload className="size-3.5" />
          {actionEntries.length > 1 ? 'Download selected' : 'Download'}
        </DropdownMenuItem>
      )}
      {(onRename || onMove || onDelete) && <DropdownMenuSeparator />}
      {onRename && (
        <DropdownMenuItem onClick={() => onRename(entry)}>
          <LuPencil className="size-3.5" />
          Rename
        </DropdownMenuItem>
      )}
      {onMove && (
        <DropdownMenuItem onClick={() => onMove(moveItems)}>
          <LuFolderInput className="size-3.5" />
          Move…
        </DropdownMenuItem>
      )}
      {onDelete && (
        <DropdownMenuItem
          variant="destructive"
          onClick={() => onDelete(actionEntries)}
        >
          <LuTrash2 className="size-3.5" />
          Delete
          <DropdownMenuShortcut>Del</DropdownMenuShortcut>
        </DropdownMenuItem>
      )}
    </>
  )
}
