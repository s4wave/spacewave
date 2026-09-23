import {
  lazy,
  Suspense,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { LuPlay, LuRotateCcw, LuX } from 'react-icons/lu'

import { useAppHref } from '@s4wave/app/prerender/StaticContext.js'
import { LoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'
import { cn } from '@s4wave/web/style/utils.js'

import type { DemoId } from './demos.js'

const LiveDemoApp = lazy(async () => {
  const { LiveDemoApp } = await import('./LiveDemoApp.js')
  return { default: LiveDemoApp }
})

// UseCaseDemoProps configures a use-case demo frame.
export interface UseCaseDemoProps {
  demo: DemoId
  live: boolean
  label: string
  poster: ReactNode
  suggestions: string[]
}

// UseCaseDemo frames a use-case demo. Until the visitor starts it, the frame
// shows a static poster of the seeded Space; the live route mounts the real
// app on an in-memory World in the same frame.
export function UseCaseDemo({
  demo,
  live,
  label,
  poster,
  suggestions,
}: UseCaseDemoProps) {
  const startHref = useAppHref(`/landing/${demo}/live`)
  const stopHref = useAppHref(`/landing/${demo}`)
  const instanceId = useId()
  const [run, setRun] = useState(0)
  const frameRef = useRef<HTMLDivElement>(null)

  // Bring the frame into view when the live route opens.
  useEffect(() => {
    if (live) frameRef.current?.scrollIntoView({ block: 'center' })
  }, [live])

  return (
    <section
      aria-label={label}
      className="relative z-10 mx-auto flex w-full max-w-6xl flex-col gap-6 px-4 @lg:px-8 @5xl:flex-row"
    >
      <div
        ref={frameRef}
        className="border-foreground/10 bg-background flex h-136 min-w-0 flex-1 flex-col overflow-hidden rounded-xl border shadow-2xl shadow-black/30 @lg:h-152"
      >
        <div className="border-foreground/10 bg-background-landing/60 flex h-10 shrink-0 items-center gap-3 border-b px-3 text-xs">
          <span
            className={cn(
              'size-2 rounded-full',
              live ? 'bg-brand animate-pulse' : 'bg-foreground/25',
            )}
          />
          <span className="text-foreground font-medium">{label}</span>
          <span className="text-foreground-alt hidden @md:inline">
            {live ? 'Running in memory in this tab' : 'Preview'}
          </span>
          {live && (
            <div className="ml-auto flex items-center gap-1">
              <button
                type="button"
                onClick={() => setRun((value) => value + 1)}
                className="text-foreground-alt hover:text-foreground hover:bg-foreground/5 flex items-center gap-1.5 rounded px-2 py-1 transition-colors"
              >
                <LuRotateCcw className="size-3.5" />
                Reset
              </button>
              <a
                href={stopHref}
                className="text-foreground-alt hover:text-foreground hover:bg-foreground/5 flex items-center gap-1.5 rounded px-2 py-1 no-underline transition-colors"
              >
                <LuX className="size-3.5" />
                Stop
              </a>
            </div>
          )}
        </div>

        {live ? (
          <Suspense
            fallback={
              <LoadingScreen
                view={{ state: 'loading', title: 'Preparing your workspace' }}
              />
            }
          >
            <LiveDemoApp
              key={run}
              demo={demo}
              appId={`landing:${demo}:${instanceId}:${run}`}
            />
          </Suspense>
        ) : (
          <div className="relative min-h-0 flex-1">
            <div aria-hidden className="h-full opacity-60 select-none">
              {poster}
            </div>
            <div className="bg-background/40 absolute inset-0 flex flex-col items-center justify-center gap-3 px-6 text-center backdrop-blur-xs">
              <a
                href={startHref}
                data-landing-live-start
                className="bg-brand text-background hover:bg-brand/90 flex items-center gap-2 rounded-md px-5 py-2.5 text-sm font-semibold no-underline shadow-lg transition duration-300 select-none hover:-translate-y-0.5"
              >
                <LuPlay className="size-4" />
                Try it live
              </a>
              <p className="text-foreground-alt max-w-xs text-xs leading-relaxed">
                Starts a real Spacewave workspace in this tab. It is kept in
                memory and forgotten when you leave.
              </p>
            </div>
          </div>
        )}
      </div>

      <aside className="flex flex-col gap-4 @5xl:w-68 @5xl:shrink-0 @5xl:pt-12">
        <h2 className="text-foreground text-sm font-semibold tracking-wide uppercase">
          Try this
        </h2>
        <ol className="flex flex-col gap-3">
          {suggestions.map((suggestion, index) => (
            <li
              key={suggestion}
              className="text-foreground-alt flex gap-3 text-sm leading-relaxed"
            >
              <span className="border-brand/40 text-brand flex size-5 shrink-0 items-center justify-center rounded-full border text-xs font-semibold">
                {index + 1}
              </span>
              <span>{suggestion}</span>
            </li>
          ))}
        </ol>
        <p className="text-foreground-alt/70 border-foreground/10 border-t pt-4 text-xs leading-relaxed">
          Reset starts the demo over. Nothing here reaches your saved Spaces.
        </p>
      </aside>
    </section>
  )
}
