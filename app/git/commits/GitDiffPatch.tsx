import { useMemo } from 'react'
import { PatchDiff, type PatchDiffProps } from '@pierre/diffs/react'

import { cn } from '@s4wave/web/style/utils.js'

export interface GitDiffPatchProps {
  patch: string | undefined
  className?: string
}

export function GitDiffPatch({ patch, className }: GitDiffPatchProps) {
  const options = useMemo<NonNullable<PatchDiffProps<undefined>['options']>>(
    () => ({
      diffStyle: 'unified',
      diffIndicators: 'classic',
      hunkSeparators: 'line-info-basic',
      overflow: 'wrap',
      themeType: 'dark',
      disableFileHeader: true,
    }),
    [],
  )

  if (!patch) return null

  return (
    <div className={cn('overflow-hidden', className)}>
      <PatchDiff
        patch={patch}
        disableWorkerPool
        className="git-diff-patch"
        options={options}
      />
    </div>
  )
}
