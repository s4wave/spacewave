import type { ChangeEvent, ComponentProps, RefObject } from 'react'

import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import {
  UnixFSContextMenu,
  type ContextMenuState,
} from './UnixFSContextMenu.js'
import { UnixFSMoveDialog } from './UnixFSMoveDialog.js'
import type { UnixFSMoveItem } from './move.js'

type UnixFSContextMenuProps = ComponentProps<typeof UnixFSContextMenu>

interface UnixFSBrowserDialogsProps {
  contextMenuProps: UnixFSContextMenuProps
  fileInputRef: RefObject<HTMLInputElement | null>
  onFileInputChange: (event: ChangeEvent<HTMLInputElement>) => void
  deleteTargets: FileEntry[] | null
  onCancelDelete: () => void
  onConfirmDelete: () => Promise<void>
  moveRootHandle: FSHandle | null | undefined
  moveDialogItems: UnixFSMoveItem[] | null
  onCancelMove: () => void
  onConfirmMove: (destinationPath: string) => Promise<void>
}

export function UnixFSBrowserDialogs({
  contextMenuProps,
  fileInputRef,
  onFileInputChange,
  deleteTargets,
  onCancelDelete,
  onConfirmDelete,
  moveRootHandle,
  moveDialogItems,
  onCancelMove,
  onConfirmMove,
}: UnixFSBrowserDialogsProps) {
  return (
    <>
      <UnixFSContextMenu {...contextMenuProps} />
      <input
        ref={fileInputRef}
        type="file"
        multiple
        data-testid="unixfs-upload-input"
        className="hidden"
        onChange={onFileInputChange}
      />
      <Dialog
        open={deleteTargets !== null}
        onOpenChange={(open) => {
          if (!open) onCancelDelete()
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              Delete {deleteTargets?.length === 1 ? 'item' : 'items'}
            </DialogTitle>
            <DialogDescription>
              {deleteTargets?.length === 1
                ? `Are you sure you want to delete "${deleteTargets[0].name}"?`
                : `Are you sure you want to delete ${deleteTargets?.length ?? 0} items?`}{' '}
              This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <button
              type="button"
              onClick={onCancelDelete}
              className="text-foreground-alt hover:text-foreground h-7 rounded-md px-3 text-xs transition-colors"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => void onConfirmDelete()}
              className="border-destructive/30 bg-destructive/10 hover:border-destructive/50 hover:bg-destructive/15 text-foreground h-7 rounded-md border px-3 text-xs font-medium transition-all duration-150"
            >
              Delete
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {moveDialogItems && moveRootHandle ? (
        <UnixFSMoveDialog
          rootHandle={moveRootHandle}
          moveItems={moveDialogItems}
          onOpenChange={(open) => {
            if (!open) onCancelMove()
          }}
          onConfirm={onConfirmMove}
        />
      ) : null}
    </>
  )
}

export type { ContextMenuState }
