import { LuRefreshCw } from 'react-icons/lu'

import { useRenderDelay } from '@s4wave/app/loading/useRenderDelay.js'
import { cn } from '@s4wave/web/style/utils.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { LoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'

import {
  spaceMountStageIndex,
  spaceMountStages,
  type SpaceMountStage,
} from './spaceMountStage.js'

interface SpaceMountingScreenProps {
  // stage is the current phase of the mount used to drive the stepper.
  stage: SpaceMountStage
  // detail is the live status line shown under the title. Updates as the
  // backing watch advances through stages.
  detail: string
  // title overrides the default "Opening your space" status.
  title?: string
  // onBack renders a floating Back button in the top-left when provided.
  onBack?: () => void
  // onRetry renders a Retry button below the stepper, gated on a short
  // delay so fast loads never flash a Retry CTA.
  onRetry?: () => void
}

const RETRY_DELAY_MS = 5_000

// SpaceMountingScreen presents watched mount progress and delayed recovery
// actions using the same composition as browser and quickstart startup.
export function SpaceMountingScreen({
  stage,
  detail,
  title = 'Opening your space',
  onBack,
  onRetry,
}: SpaceMountingScreenProps) {
  const allowRetry = useRenderDelay(RETRY_DELAY_MS)
  return (
    <LoadingScreen
      view={{ state: 'active', title, detail }}
      topLeftSlot={
        onBack ? (
          <BackButton floating onClick={onBack}>
            Back
          </BackButton>
        ) : undefined
      }
      footer={
        onRetry && allowRetry ? (
          <div className="mt-2 flex justify-center">
            <DashboardButton
              icon={<LuRefreshCw className="size-3.5" />}
              onClick={onRetry}
            >
              Retry
            </DashboardButton>
          </div>
        ) : null
      }
    >
      <SpaceMountStepper current={stage} />
    </LoadingScreen>
  )
}

// SpaceMountStepper presents the four watched mount phases.
function SpaceMountStepper({ current }: { current: SpaceMountStage }) {
  const currentIndex = spaceMountStageIndex(current)
  return (
    <div className="flex items-start justify-center gap-5">
      {spaceMountStages.map((entry, i) => {
        const isComplete = i < currentIndex
        const isActive = i === currentIndex
        return (
          <div key={entry.id} className="flex flex-col items-center gap-2">
            <span
              className={cn(
                'relative size-2 rounded-full transition-colors duration-300',
                isComplete && 'bg-brand/60',
                isActive && 'bg-brand',
                !isComplete && !isActive && 'bg-foreground/15',
              )}
              aria-hidden="true"
            >
              {isActive ? (
                <span className="bg-brand/30 inset-logo-outline absolute animate-ping rounded-full motion-reduce:animate-none" />
              ) : null}
            </span>
            <span
              className={cn(
                'micro-text font-medium tracking-widest uppercase transition-colors select-none',
                isComplete && 'text-foreground-alt/55',
                isActive && 'text-foreground',
                !isComplete && !isActive && 'text-foreground-alt/35',
              )}
            >
              {entry.label}
            </span>
          </div>
        )
      })}
    </div>
  )
}
