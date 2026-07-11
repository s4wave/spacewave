import { LuCircleAlert, LuMonitor, LuRotateCw } from 'react-icons/lu'

import { PhaseChecklist } from '@s4wave/app/session/setup/PhaseChecklist.js'
import { ProgressBar } from '@s4wave/web/ui/loading/ProgressBar.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

export type VmCreationStage = 'fetching' | 'copying' | 'creating' | 'ready'

export interface VmCreationProgress {
  stage: VmCreationStage
  blocksSeen?: bigint
  blocksCopied?: bigint
  blocksWritten?: bigint
  logicalSourceBytes?: bigint
}

interface VmCreationProgressScreenProps {
  progress: VmCreationProgress
  vmName: string
  includesCdnCopy: boolean
  error?: string
  onRetry?: () => void
}

const cdnStages: ReadonlyArray<{
  stage: VmCreationStage
  label: string
}> = [
  { stage: 'fetching', label: 'Fetching image from CDN' },
  { stage: 'copying', label: 'Copying blocks' },
  { stage: 'creating', label: 'Creating VM' },
  { stage: 'ready', label: 'Ready' },
]

const localStages = cdnStages.slice(2)

function formatBytes(bytes: bigint): string {
  if (bytes < 1024n) return `${bytes} B`
  const units = [
    { size: 1024n ** 3n, label: 'GB' },
    { size: 1024n ** 2n, label: 'MB' },
    { size: 1024n, label: 'KB' },
  ]
  for (const unit of units) {
    if (bytes < unit.size) continue
    const tenths = (bytes * 10n) / unit.size
    return `${tenths / 10n}.${tenths % 10n} ${unit.label}`
  }
  return `${bytes} B`
}

function getProgressDetail(
  progress: VmCreationProgress,
  vmName: string,
): string {
  switch (progress.stage) {
    case 'fetching':
      return 'Resolving the image and destination Space.'
    case 'copying': {
      const seen = progress.blocksSeen ?? 0n
      const copied = progress.blocksCopied ?? 0n
      const written = progress.blocksWritten ?? 0n
      const bytes = progress.logicalSourceBytes ?? 0n
      if (seen === 0n && copied === 0n && bytes === 0n) {
        return 'Finding and copying the image blocks.'
      }
      return `${seen} seen · ${copied} copied · ${written} written · ${formatBytes(bytes)}`
    }
    case 'creating':
      return 'Image ready. Writing the VM object.'
    case 'ready':
      return `${vmName || 'VM'} is ready. Opening it now.`
  }
}

// VmCreationProgressScreen renders the event-driven CDN copy and VM creation
// stages without implying a byte fraction the copy owner cannot provide.
export function VmCreationProgressScreen({
  progress,
  vmName,
  includesCdnCopy,
  error,
  onRetry,
}: VmCreationProgressScreenProps) {
  const stages = includesCdnCopy ? cdnStages : localStages
  const activeIndex = Math.max(
    0,
    stages.findIndex((item) => item.stage === progress.stage),
  )
  const phases = stages.map((item, index) => ({
    label: item.label,
    done: index < activeIndex || progress.stage === 'ready',
    active: !error && index === activeIndex && progress.stage !== 'ready',
  }))
  const title = error
    ? 'VM could not be created'
    : progress.stage === 'ready'
      ? 'VM ready'
      : 'Creating virtual machine'

  return (
    <div className="bg-background flex min-h-[24rem] w-full flex-1 items-center justify-center p-6">
      <div className="border-foreground/10 bg-foreground/[0.02] w-full max-w-sm rounded-xl border p-5 shadow-sm">
        <div className="flex items-center gap-3">
          <div className="bg-brand/10 text-brand flex size-9 shrink-0 items-center justify-center rounded-lg">
            {error ? (
              <LuCircleAlert
                className="text-destructive size-4"
                aria-hidden="true"
              />
            ) : progress.stage === 'ready' ? (
              <LuMonitor className="size-4" aria-hidden="true" />
            ) : (
              <Spinner className="text-brand" />
            )}
          </div>
          <div className="min-w-0">
            <h2 className="text-foreground text-sm font-medium">{title}</h2>
            <p
              className="text-foreground-alt mt-0.5 text-xs leading-relaxed"
              aria-live="polite"
            >
              {error ?? getProgressDetail(progress, vmName)}
            </p>
          </div>
        </div>

        <div className="mt-5">
          <ProgressBar
            value={progress.stage === 'ready' && !error ? 100 : undefined}
            indeterminate={progress.stage !== 'ready' && !error}
          />
        </div>

        <PhaseChecklist phases={phases} className="mt-5 px-0" />

        {error && onRetry ? (
          <button
            type="button"
            onClick={onRetry}
            className="border-foreground/10 bg-foreground/5 hover:bg-foreground/10 text-foreground mt-5 inline-flex h-9 items-center gap-2 rounded-md border px-3 text-xs font-medium transition-colors motion-reduce:transition-none"
          >
            <LuRotateCw className="size-3.5" aria-hidden="true" />
            Retry
          </button>
        ) : null}
      </div>
    </div>
  )
}
