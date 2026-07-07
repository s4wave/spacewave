import type { DragEvent } from 'react'
import { PathInput } from '../../ui/path/PathInput.js'

interface PathBarProps {
  path: string
  onPathChange?: (path: string) => void
  onNavigate?: (path: string) => void
  onPathTargetDragOver?: (
    path: string,
    event: DragEvent<HTMLElement>,
  ) => boolean
  onPathTargetDrop?: (path: string, event: DragEvent<HTMLElement>) => void
}

export function PathBar({
  path,
  onPathChange,
  onNavigate,
  onPathTargetDragOver,
  onPathTargetDrop,
}: PathBarProps) {
  return (
    <PathInput
      path={path}
      onPathChange={onPathChange}
      onNavigate={onNavigate}
      onPathTargetDragOver={onPathTargetDragOver}
      onPathTargetDrop={onPathTargetDrop}
    />
  )
}
