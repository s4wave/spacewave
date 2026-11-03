import * as stylex from '@stylexjs/stylex'
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

/** PathBar shows the file path with touch-sized breadcrumbs on phones. */
export function PathBar({
  sx,
  path,
  onPathChange,
  onNavigate,
  onPathTargetDragOver,
  onPathTargetDrop,
}: PathBarProps & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <PathInput
      sx={sx}
      path={path}
      mobileTouchTargets
      onPathChange={onPathChange}
      onNavigate={onNavigate}
      onPathTargetDragOver={onPathTargetDragOver}
      onPathTargetDrop={onPathTargetDrop}
    />
  )
}
