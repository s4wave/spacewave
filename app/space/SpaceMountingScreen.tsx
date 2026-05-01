import { LuRefreshCw } from 'react-icons/lu'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
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
  // title overrides the default screen title. Defaults to "Mounting your
  // space" so the user has a calm, confident anchor while the screen waits.
  title?: string
  // onBack renders a floating Back button in the top-left when provided.
  onBack?: () => void
  // onRetry renders a Retry button below the stepper, gated on a short
  // delay so fast loads never flash a Retry CTA.
  onRetry?: () => void
}

const RETRY_DELAY_MS = 5_000

// SpaceMountingScreen renders the route-level loader shown while a space is
// being mounted. Uses the shared LoadingScreen primitive for the animated
// logo and shine border, then layers a stage stepper, optional Back, and
// optional Retry on top.
export function SpaceMountingScreen({
  stage,
  detail,
  title = 'Mounting your space',
  onBack,
  onRetry,
}: SpaceMountingScreenProps) {
  const allowRetry = useRenderDelay(RETRY_DELAY_MS)
  return (
    <LoadingScreen
      view={{ state: 'active', title, detail }}
      logo={<AnimatedLogo followMouse={false} />}
      containerClassName="bg-background relative flex h-full min-h-[28rem] w-full flex-col items-center justify-center overflow-hidden"
      topLeftSlot={
        onBack ?
          <BackButton floating onClick={onBack}>
            Back
          </BackButton>
        : undefined
      }
    >
      <SpaceMountStepper current={stage} />
      {onRetry && allowRetry ?
        <div className="mt-2 flex justify-center">
          <DashboardButton
            icon={<LuRefreshCw className="h-3.5 w-3.5" />}
            onClick={onRetry}
          >
            Retry
          </DashboardButton>
        </div>
      : null}
    </LoadingScreen>
  )
}

// SpaceMountStepper renders the four stage dots and labels under the title.
// Active dot pulses brand color; completed dots are filled muted brand;
// future dots stay neutral. Read-only -- the stepper never accepts clicks.
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
                'relative h-2 w-2 rounded-full transition-colors duration-300',
                isComplete && 'bg-brand/60',
                isActive && 'bg-brand',
                !isComplete && !isActive && 'bg-foreground/15',
              )}
              aria-hidden="true"
            >
              {isActive ?
                <span className="bg-brand/30 absolute inset-[-6px] animate-ping rounded-full" />
              : null}
            </span>
            <span
              className={cn(
                'text-[0.6rem] font-medium tracking-widest uppercase select-none transition-colors',
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
