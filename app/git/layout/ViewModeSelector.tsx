import { useMemo } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

import type { ViewMode } from './route.js'

// ViewModeSelectorProps are props for the ViewModeSelector component.
export interface ViewModeSelectorProps {
  mode: ViewMode
  onModeChange: (mode: ViewMode) => void
  hasReadme: boolean
  availableModes?: ViewMode[]
}

// defaultTabs defines the label for each view mode.
const modeLabels: Record<ViewMode, string> = {
  files: 'Files',
  readme: 'README',
  log: 'Log',
  commit: 'Commit',
  workdir: 'Workdir',
  changes: 'Changes',
}

// ViewModeSelector renders a tab bar for switching content panels.
export function ViewModeSelector({
  mode,
  onModeChange,
  hasReadme,
  availableModes,
}: ViewModeSelectorProps) {
  const tabs = useMemo(() => {
    if (availableModes) {
      return availableModes
        .filter((m) => m !== 'readme' || hasReadme)
        .map((m) => ({ key: m, label: modeLabels[m] }))
    }
    const result: { key: ViewMode; label: string }[] = [
      { key: 'files', label: 'Files' },
    ]
    if (hasReadme) {
      result.push({ key: 'readme', label: 'README' })
    }
    result.push({ key: 'log', label: 'Log' })
    return result
  }, [availableModes, hasReadme])

  return (
    <div className="border-foreground/8 flex items-center gap-0.5 border-b px-2 py-0.5">
      {tabs.map((tab) => (
        <button
          key={tab.key}
          className={cn(
            'rounded px-2 py-0.5 text-xs select-none',
            mode === tab.key ?
              'text-foreground bg-white/[0.08] font-medium'
            : 'text-foreground-alt hover:text-foreground hover:bg-white/[0.03]',
          )}
          onClick={() => onModeChange(tab.key)}
        >
          {tab.label}
        </button>
      ))}
    </div>
  )
}
