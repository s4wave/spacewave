import { PathInput } from '../../ui/path/index.js'

interface PathBarProps {
  path: string
  onPathChange?: (path: string) => void
  onNavigate?: (path: string) => void
}

export function PathBar({ path, onPathChange, onNavigate }: PathBarProps) {
  return (
    <PathInput
      path={path}
      onPathChange={onPathChange}
      onNavigate={onNavigate}
    />
  )
}
