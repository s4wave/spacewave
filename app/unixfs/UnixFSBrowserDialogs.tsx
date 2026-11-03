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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { display: 'none' },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
})

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
        aria-label="Upload files"
        multiple
        data-testid="unixfs-upload-input"
        sx={stylexStyles.rule1}
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
              sx={stylexStyles.rule2}
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => void onConfirmDelete()}
              sx={stylexStyles.rule3}
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
