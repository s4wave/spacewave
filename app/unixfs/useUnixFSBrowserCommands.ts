import { useCallback } from 'react'

import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import { useCommand } from '@s4wave/web/command/useCommand.js'
import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'

interface UnixFSBrowserCommandsOptions {
  selectedEntries: FileEntry[]
  canGoBack: boolean
  canGoForward: boolean
  canGoUp: boolean
  onNewFile: () => void
  onNewFolder: () => void
  onUploadFiles: () => void
  onOpen: (entries: FileEntry[]) => void
  onRename: (entry: FileEntry) => void
  onDownload: (entries: FileEntry[]) => void
  onDelete: (entries: FileEntry[]) => void
  onBack: () => void
  onForward: () => void
  onUp: () => void
}

export function useUnixFSBrowserCommands({
  selectedEntries,
  canGoBack,
  canGoForward,
  canGoUp,
  onNewFile,
  onNewFolder,
  onUploadFiles,
  onOpen,
  onRename,
  onDownload,
  onDelete,
  onBack,
  onForward,
  onUp,
}: UnixFSBrowserCommandsOptions) {
  const isTabActive = useIsTabActive()
  const hasSelection = selectedEntries.length > 0
  const hasSingleSelection = selectedEntries.length === 1

  const openSelected = useCallback(() => {
    if (hasSelection) onOpen(selectedEntries)
  }, [hasSelection, onOpen, selectedEntries])

  const renameSelected = useCallback(() => {
    if (hasSingleSelection) onRename(selectedEntries[0])
  }, [hasSingleSelection, onRename, selectedEntries])

  const downloadSelected = useCallback(() => {
    if (hasSelection) onDownload(selectedEntries)
  }, [hasSelection, onDownload, selectedEntries])

  const deleteSelected = useCallback(() => {
    if (hasSelection) onDelete(selectedEntries)
  }, [hasSelection, onDelete, selectedEntries])

  useCommand({
    commandId: 'spacewave.file.new-file',
    label: 'New File',
    menuPath: 'File/New File',
    menuGroup: 2,
    menuOrder: 1,
    active: isTabActive,
    handler: onNewFile,
  })

  useCommand({
    commandId: 'spacewave.file.new-folder',
    label: 'New Folder',
    menuPath: 'File/New Folder',
    menuGroup: 2,
    menuOrder: 2,
    active: isTabActive,
    handler: onNewFolder,
  })

  useCommand({
    commandId: 'spacewave.file.upload',
    label: 'Upload',
    menuPath: 'File/Upload',
    menuGroup: 2,
    menuOrder: 3,
    active: isTabActive,
    handler: onUploadFiles,
  })

  useCommand({
    commandId: 'spacewave.file.open',
    label: 'Open Selected',
    description: 'Open the selected file or directory',
    menuPath: 'File/Open Selected',
    menuGroup: 1,
    menuOrder: 1,
    active: isTabActive,
    enabled: hasSelection,
    handler: openSelected,
  })

  useCommand({
    commandId: 'spacewave.file.rename',
    label: 'Rename',
    description: 'Rename the selected file or directory',
    keybinding: 'F2',
    menuPath: 'File/Rename',
    menuGroup: 3,
    menuOrder: 1,
    active: isTabActive,
    enabled: hasSingleSelection,
    handler: renameSelected,
  })

  useCommand({
    commandId: 'spacewave.file.download',
    label: 'Download',
    description: 'Download the selected file or selection',
    menuPath: 'File/Download',
    menuGroup: 3,
    menuOrder: 2,
    active: isTabActive,
    enabled: hasSelection,
    handler: downloadSelected,
  })

  useCommand({
    commandId: 'spacewave.file.delete',
    label: 'Delete',
    menuPath: 'Edit/Delete',
    menuGroup: 40,
    menuOrder: 1,
    active: isTabActive,
    enabled: hasSelection,
    handler: deleteSelected,
  })

  useCommand({
    commandId: 'spacewave.nav.back',
    label: 'Navigate Back',
    keybinding: 'Alt+ArrowLeft',
    menuPath: 'File/Navigate Back',
    menuGroup: 30,
    menuOrder: 1,
    active: isTabActive,
    enabled: canGoBack,
    handler: onBack,
  })

  useCommand({
    commandId: 'spacewave.nav.forward',
    label: 'Navigate Forward',
    keybinding: 'Alt+ArrowRight',
    menuPath: 'File/Navigate Forward',
    menuGroup: 30,
    menuOrder: 2,
    active: isTabActive,
    enabled: canGoForward,
    handler: onForward,
  })

  useCommand({
    commandId: 'spacewave.nav.up',
    label: 'Navigate Up',
    keybinding: 'Alt+ArrowUp',
    menuPath: 'File/Navigate Up',
    menuGroup: 30,
    menuOrder: 3,
    active: isTabActive,
    enabled: canGoUp,
    handler: onUp,
  })
}
