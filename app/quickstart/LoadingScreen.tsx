import { LoadingScreen as BaseLoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { PhaseChecklist } from '@s4wave/app/session/setup/PhaseChecklist.js'

import type {
  QuickstartProgressState,
  QuickstartProgressStep,
} from './create.js'

const progressLabels: Array<{ step: QuickstartProgressStep; label: string }> = [
  { step: 'session', label: 'Local session' },
  { step: 'space', label: 'Space' },
  { step: 'frame', label: 'Frame-ready' },
  { step: 'content', label: 'Content-ready' },
]

function getProgressLabels(quickstartId: string) {
  if (quickstartId === 'local') return progressLabels.slice(0, 1)
  return progressLabels
}

function buildQuickstartProgress(
  quickstartId: string,
): QuickstartProgressState {
  const labels = getProgressLabels(quickstartId)
  return {
    step: 'session',
    stepIndex: 1,
    stepCount: labels.length,
    detail: `Setting up ${quickstartId}`,
  }
}

// LoadingScreen is the full-screen boot surface used while a quickstart
// initializes a session. Drives the LoadingScreen primitive with a dynamic
// quickstart-id-driven view.
export function LoadingScreen({
  quickstartId,
  progress,
}: {
  quickstartId: string
  progress?: QuickstartProgressState | null
}) {
  const current = progress ?? buildQuickstartProgress(quickstartId)
  const labels = getProgressLabels(quickstartId)
  const activeIndex = Math.max(
    0,
    labels.findIndex((item) => item.step === current.step),
  )
  const phases = labels.map((item, index) => ({
    label: item.label,
    done: index < activeIndex,
    active: index === activeIndex,
  }))

  return (
    <BaseLoadingScreen
      view={{
        state: 'active',
        title: 'Initializing Spacewave',
        detail: current.detail,
        progress: (activeIndex + 0.5) / labels.length,
      }}
      logo={<AnimatedLogo followMouse={false} />}
    >
      <PhaseChecklist phases={phases} className="w-64" />
    </BaseLoadingScreen>
  )
}
