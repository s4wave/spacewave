import type { ReactNode } from 'react'
import { LuArrowLeft, LuRotateCw } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { ShineBorder } from '@s4wave/web/ui/shine-border.js'

import { ProgressBar } from './ProgressBar.js'
import { Spinner } from './Spinner.js'
import type { LoadingView } from './types.js'
import { useReducedMotion } from './useReducedMotion.js'

interface LoadingScreenProps {
  view: LoadingView
  // logo is an optional React node rendered above the title (e.g. an animated
  // app logo). When omitted, a simple branded Spinner is rendered in its
  // place so callers that only want the state machine can omit the app logo.
  logo?: ReactNode
  // showShineBorder wraps the screen in the animated gradient border. Defaults
  // to true for full-app boot screens; disable for panel-level full-screen
  // overlays.
  showShineBorder?: boolean
  // topLeftSlot renders an absolutely positioned overlay in the top-left
  // corner of the screen, above the shine border. Used for floating Back
  // buttons on route-level loaders.
  topLeftSlot?: ReactNode
  // children render after the title/detail/progress block, inside the same
  // centered column. Use for stage steppers, retry buttons, or any extra
  // content specific to the surface.
  children?: ReactNode
  // containerClassName overrides the default outer wrapper classes. Defaults
  // to the full-viewport boot wrapper. Pass `h-full w-full` (or similar) to
  // fit the screen inside an existing layout frame.
  containerClassName?: string
}

const defaultContainerClassName =
  'bg-background relative flex min-h-screen w-full flex-col items-center justify-center overflow-hidden'

// LoadingScreen is the full-viewport boot surface. Keeps the animated logo
// slot and shine border while driving title / detail / progress from a
// LoadingView. Used by app boot, quickstart init, and any other screen that
// takes over the entire viewport during load.
export function LoadingScreen({
  view,
  logo,
  showShineBorder = true,
  topLeftSlot,
  children,
  containerClassName,
}: LoadingScreenProps) {
  const reducedMotion = useReducedMotion()

  return (
    <div
      className={cn(defaultContainerClassName, containerClassName)}
      data-sw-reduced-motion={reducedMotion ? 'true' : undefined}
    >
      {showShineBorder && !reducedMotion ?
        <div className="pointer-events-none absolute inset-0">
          <ShineBorder
            borderWidth={2}
            duration={20}
            shineColor={[
              'var(--color-logo-blue)',
              'var(--color-logo-pink)',
              'var(--color-logo-purple)',
              'var(--color-logo-blue)',
            ]}
            className="rounded-br-[12px] rounded-bl-[12px]"
          />
        </div>
      : null}

      {topLeftSlot ?? null}

      <div className="relative z-10 flex flex-col items-center gap-y-6">
        {logo ?
          <div className="mb-4">{logo}</div>
        : <div className="bg-brand/10 mb-4 flex size-12 items-center justify-center rounded-xl">
            <Spinner size="xl" className="text-brand" />
          </div>
        }

        <div className="space-y-2 text-center" aria-live="polite">
          <h1 className="text-foreground text-2xl font-semibold tracking-tight select-none">
            {view.title}
          </h1>

          {view.detail ?
            <p className="text-foreground-alt/70 text-sm select-none">
              {view.detail}
            </p>
          : null}

          {view.progress !== undefined ?
            <div className="mx-auto mt-4 w-64">
              <ProgressBar
                value={view.progress * 100}
                rate={view.rate?.down ?? view.rate?.up}
              />
            </div>
          : null}

          {view.error ?
            <p className="bg-destructive/5 border-destructive/15 text-destructive mx-auto mt-4 max-w-xs rounded-md border px-3 py-2 text-xs leading-relaxed">
              {view.error}
            </p>
          : null}

          {view.onRetry || view.onCancel ?
            <div className="mt-4 flex flex-wrap items-center justify-center gap-2">
              {view.onRetry ?
                <button
                  type="button"
                  onClick={view.onRetry}
                  className="border-foreground/10 bg-foreground/5 hover:bg-foreground/10 hover:border-foreground/15 text-foreground inline-flex h-9 items-center gap-2 rounded-md border px-3 text-sm font-medium transition-colors motion-reduce:transition-none"
                >
                  <LuRotateCw className="size-4" aria-hidden="true" />
                  <span>Retry</span>
                </button>
              : null}
              {view.onCancel ?
                <button
                  type="button"
                  onClick={view.onCancel}
                  className="text-foreground-alt hover:text-foreground inline-flex h-9 items-center gap-2 rounded-md px-3 text-sm font-medium transition-colors motion-reduce:transition-none"
                >
                  <LuArrowLeft className="size-4" aria-hidden="true" />
                  <span>Back</span>
                </button>
              : null}
            </div>
          : null}
        </div>

        {children ?? null}
      </div>
    </div>
  )
}
