import type { DragEvent, KeyboardEvent, ReactNode } from 'react'

import { Toolbar } from '@s4wave/web/editors/file-browser/Toolbar.js'

interface UnixFSBrowserShellProps {
  currentPath: string
  children: ReactNode
  onPathChange: (path: string) => void
  onBack: () => void
  onForward: () => void
  onUp: () => void
  canGoBack: boolean
  canGoForward: boolean
  canGoUp: boolean
  upDropPath?: string
  interactive?: boolean
  onKeyDown?: (event: KeyboardEvent<HTMLDivElement>) => void
  onNewFolder?: () => void
  onUploadFiles?: () => void
  onPathTargetDragOver?: (
    path: string,
    event: DragEvent<HTMLElement>,
  ) => boolean
  onPathTargetDrop?: (path: string, event: DragEvent<HTMLElement>) => void
}

export function UnixFSBrowserShell({
  currentPath,
  children,
  onPathChange,
  onBack,
  onForward,
  onUp,
  canGoBack,
  canGoForward,
  canGoUp,
  upDropPath,
  interactive = false,
  onKeyDown,
  onNewFolder,
  onUploadFiles,
  onPathTargetDragOver,
  onPathTargetDrop,
}: UnixFSBrowserShellProps) {
  return (
    <div
      role={interactive ? 'region' : undefined}
      aria-label={interactive ? 'File browser' : undefined}
      data-testid="unixfs-browser"
      className="flex h-full w-full flex-col overflow-hidden"
      onKeyDown={onKeyDown}
    >
      <Toolbar
        currentPath={currentPath}
        onPathChange={onPathChange}
        upDropPath={upDropPath}
        onNavigate={onPathChange}
        onBack={onBack}
        onForward={onForward}
        onUp={onUp}
        canGoBack={canGoBack}
        canGoForward={canGoForward}
        canGoUp={canGoUp}
        onNewFolder={onNewFolder}
        onUploadFiles={onUploadFiles}
        onPathTargetDragOver={onPathTargetDragOver}
        onPathTargetDrop={onPathTargetDrop}
      />
      {children}
    </div>
  )
}
