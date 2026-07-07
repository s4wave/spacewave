import type { DragEvent, MouseEvent, ReactNode } from 'react'
import { LuUpload } from 'react-icons/lu'

interface UnixFSBrowserDropSurfaceProps {
  children: ReactNode
  isDragging: boolean
  floatingDiagnostics?: ReactNode
  onContextMenu: (event: MouseEvent<HTMLDivElement>) => void
  onDragOver: (event: DragEvent<HTMLDivElement>) => void
  onDragLeave: (event: DragEvent<HTMLDivElement>) => void
  onDrop: (event: DragEvent<HTMLDivElement>) => void
}

export function UnixFSBrowserDropSurface({
  children,
  isDragging,
  floatingDiagnostics,
  onContextMenu,
  onDragOver,
  onDragLeave,
  onDrop,
}: UnixFSBrowserDropSurfaceProps) {
  return (
    <div
      data-testid="unixfs-upload-drop-target"
      className="bg-file-back relative flex min-h-0 flex-1 flex-col overflow-hidden"
      onContextMenu={onContextMenu}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
    >
      {children}
      {floatingDiagnostics && (
        <div className="border-foreground/5 bg-background/85 pointer-events-none absolute bottom-3 left-1/2 z-10 -translate-x-1/2 rounded-md border px-3 py-2 shadow-sm backdrop-blur">
          {floatingDiagnostics}
        </div>
      )}
      {isDragging && (
        <div className="border-brand/50 bg-brand/5 pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-md border-2 border-dashed">
          <div className="flex flex-col items-center gap-2">
            <LuUpload className="text-brand size-8" />
            <span className="text-brand text-sm font-medium">
              Drop files to upload
            </span>
          </div>
        </div>
      )}
    </div>
  )
}
