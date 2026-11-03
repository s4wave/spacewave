import { useMemo } from 'react'
import { LuCheck, LuX } from '@s4wave/web/images/icons.js'

import type { RenderEntryCallback } from '@s4wave/web/editors/file-browser/FileListEntry.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    overflow: 'hidden',
    alignItems: 'center',
    display: 'flex',
    minWidth: `calc(${themeLayout['--spacing']} * 30)`,
  },
  rule2: {
    '--tw-outline-style': 'none',
    borderColor: themeColors['--color-brand'],
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: themeColors['--color-background'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
    minWidth: 0,
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand-highlight'],
      },
    },
    flexShrink: 0,
  },
  rule4: {
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    flexShrink: 0,
  },
  rule5: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    overflow: 'hidden',
    alignItems: 'center',
    display: 'flex',
    minWidth: `calc(${themeLayout['--spacing']} * 30)`,
  },
  rule6: {
    '--tw-outline-style': 'none',
    borderColor: themeColors['--color-brand'],
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    paddingBlock: 0,
    paddingInline: themeLayout['--spacing'],
    backgroundColor: themeColors['--color-background'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
    minWidth: 0,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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
            sx={stylexStyles2.rule1}
            data-rename-actions="true"
            onClick={(event) => event.stopPropagation()}
            onMouseDown={(event) => event.stopPropagation()}
          >
            <input
              aria-label="Rename entry"
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
              sx={stylexStyles2.rule2}
              defaultValue={renameRef.current}
              onChange={(event) => {
                renameRef.current = event.target.value
              }}
              onKeyDown={(event) => {
                event.stopPropagation()
                if (event.nativeEvent.isComposing) return
                if (event.key === 'Enter') {
                  event.preventDefault()
                  void onConfirmRename()
                }
                if (event.key === 'Escape') {
                  event.preventDefault()
                  onCancelRename()
                }
              }}
              onBlur={(event) => {
                const related = event.relatedTarget as HTMLElement | null
                if (related?.closest('[data-rename-actions]')) return
                onCancelRename()
              }}
            />
            <button
              type="button"
              tabIndex={0}
              aria-label="Confirm rename"
              sx={stylexStyles2.rule3}
              onClick={(event) => {
                event.preventDefault()
                event.stopPropagation()
                void onConfirmRename()
              }}
            >
              <LuCheck sx={stylexStyles.rule1} />
            </button>
            <button
              type="button"
              tabIndex={0}
              aria-label="Cancel rename"
              sx={stylexStyles2.rule4}
              onClick={(event) => {
                event.preventDefault()
                event.stopPropagation()
                onCancelRename()
              }}
            >
              <LuX sx={stylexStyles.rule1} />
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
          sx={stylexStyles2.rule5}
          onClick={(event) => event.stopPropagation()}
          onMouseDown={(event) => event.stopPropagation()}
        >
          <input
            aria-label={placeholder}
            ref={(element) => element?.focus()}
            sx={stylexStyles2.rule6}
            value={value}
            onChange={(event) => handleNameChange(event.target.value)}
            onKeyDown={(event) => {
              event.stopPropagation()
              if (event.nativeEvent.isComposing) return
              if (event.key === 'Enter') {
                event.preventDefault()
                void handleConfirm(event.currentTarget.value)
              }
              if (event.key === 'Escape') {
                event.preventDefault()
                handleCancel()
              }
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
