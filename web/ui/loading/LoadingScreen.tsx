import type { ReactNode } from 'react'

import { ShineBorder } from '@s4wave/web/ui/shine-border.js'

import { ProgressBar } from './ProgressBar.js'
import { Spinner } from './Spinner.js'
import type { LoadingView } from './types.js'

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
}

// LoadingScreen is the full-viewport boot surface. Keeps the animated logo
// slot and shine border while driving title / detail / progress from a
// LoadingView. Used by app boot, quickstart init, and any other screen that
// takes over the entire viewport during load.
export function LoadingScreen({
  view,
  logo,
  showShineBorder = true,
}: LoadingScreenProps) {
  return (
    <div className="bg-background relative flex min-h-screen w-full flex-col items-center justify-center overflow-hidden">
      {showShineBorder ?
        <div className="absolute inset-0">
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

      <div className="relative z-10 flex flex-col items-center space-y-6">
        {logo ?
          <div className="mb-4">{logo}</div>
        : <div className="bg-brand/10 mb-4 flex h-12 w-12 items-center justify-center rounded-xl">
            <Spinner size="xl" className="text-brand" />
          </div>
        }

        <div className="space-y-2 text-center">
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
        </div>
      </div>
    </div>
  )
}
