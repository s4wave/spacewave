import type { KeyboardEvent, ReactNode } from 'react'

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
  interactive?: boolean
  onKeyDown?: (event: KeyboardEvent<HTMLDivElement>) => void
  onNewFolder?: () => void
  onUploadFiles?: () => void
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
  interactive = false,
  onKeyDown,
  onNewFolder,
  onUploadFiles,
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
        onNavigate={onPathChange}
        onBack={onBack}
        onForward={onForward}
        onUp={onUp}
        canGoBack={canGoBack}
        canGoForward={canGoForward}
        canGoUp={canGoUp}
        onNewFolder={onNewFolder}
        onUploadFiles={onUploadFiles}
      />
      {children}
    </div>
  )
}
