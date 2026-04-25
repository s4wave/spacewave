import { useCallback, useEffect, useState } from 'react'
import {
  LuArrowLeft,
  LuCircleAlert,
  LuCircleCheck,
  LuCloud,
  LuHardDrive,
  LuLoader,
} from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'

// LoadingDebug renders candidate "unified loading" primitives in one gallery.
// All primitives are inline in this file; none are extracted to web/ui/ yet.
// Once the visual treatment is approved, primitives move to web/ui/loading/.
export function LoadingDebug() {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  return (
    <div className="bg-background flex h-full w-full flex-col overflow-auto">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <button
          type="button"
          onClick={goBack}
          className="text-foreground-alt hover:text-foreground transition-colors"
          aria-label="Back"
        >
          <LuArrowLeft className="h-4 w-4" />
        </button>
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          Loading UI Prototype
        </span>
      </div>

      <div className="flex-1 overflow-auto px-4 py-3">
        <div className="mx-auto max-w-3xl space-y-6">
          <Intro />
          <SpinnerSection />
          <ProgressBarSection />
          <LoadingCardSection />
          <LoadingInlineSection />
          <LoadingScreenSection />
        </div>
      </div>
    </div>
  )
}

function Intro() {
  return (
    <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5 backdrop-blur-sm">
      <p className="text-foreground text-sm font-semibold tracking-tight">
        Unified loading primitive gallery
      </p>
      <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
        Five primitives (Spinner, ProgressBar, LoadingCard, LoadingInline,
        LoadingScreen) rendered in all states and sizes. Every surface uses the
        modern token set (brand / foreground / background-card / destructive)
        and the glass card stack (border-foreground/6, bg-background-card/30,
        backdrop-blur-sm). Skeleton is intentionally omitted: research shows
        skeleton placeholders can amplify perceived lag on fast loads. The
        current production components (LoadingSpinner, LoadingOverlay,
        LoadingCard, progress.tsx, LoadingScreen) get rewritten to match once
        the treatment is approved; skeleton.tsx and its dormant shimmer keyframe
        get removed.
      </p>
    </div>
  )
}

// -----------------------------------------------------------------------
// Section wrapper
// -----------------------------------------------------------------------

interface SectionProps {
  title: string
  description: string
  children: React.ReactNode
}

function Section({ title, description, children }: SectionProps) {
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-foreground text-sm font-semibold tracking-tight select-none">
          {title}
        </h2>
        <p className="text-foreground-alt/60 mt-0.5 text-xs">{description}</p>
      </div>
      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-4 backdrop-blur-sm">
        {children}
      </div>
    </section>
  )
}

function Label({ children }: { children: React.ReactNode }) {
  return (
    <div className="text-foreground-alt/50 text-[0.55rem] font-medium tracking-widest uppercase">
      {children}
    </div>
  )
}

// -----------------------------------------------------------------------
// 1. Spinner
// -----------------------------------------------------------------------

type SpinnerSize = 'sm' | 'md' | 'lg' | 'xl'

const spinnerSizes: Record<SpinnerSize, string> = {
  sm: 'h-3.5 w-3.5',
  md: 'h-4 w-4',
  lg: 'h-6 w-6',
  xl: 'h-8 w-8',
}

// Spinner inherits text color from parent so container state colors apply.
function Spinner({
  size = 'md',
  className,
}: {
  size?: SpinnerSize
  className?: string
}) {
  return (
    <LuLoader
      className={cn('animate-spin', spinnerSizes[size], className)}
      aria-hidden="true"
    />
  )
}

function SpinnerSection() {
  return (
    <Section
      title="Spinner"
      description="Atomic loading indicator. Inherits text color from parent; sized h-3.5 / h-4 / h-6 / h-8. Replaces LoadingSpinner and ~20 inline LuLoader animate-spin sites."
    >
      <div className="grid grid-cols-4 gap-4">
        {(['sm', 'md', 'lg', 'xl'] as SpinnerSize[]).map((size) => (
          <div key={size} className="flex flex-col items-center gap-2">
            <div className="flex h-12 items-center justify-center">
              <Spinner size={size} className="text-brand" />
            </div>
            <Label>{size}</Label>
          </div>
        ))}
      </div>
      <div className="border-foreground/8 mt-4 grid grid-cols-4 gap-4 border-t pt-4">
        {(
          [
            { tone: 'brand', cls: 'text-brand' },
            { tone: 'muted', cls: 'text-foreground-alt' },
            { tone: 'destructive', cls: 'text-destructive' },
            { tone: 'success', cls: 'text-success' },
          ] as const
        ).map(({ tone, cls }) => (
          <div key={tone} className="flex flex-col items-center gap-2">
            <div className="flex h-12 items-center justify-center">
              <Spinner size="lg" className={cls} />
            </div>
            <Label>{tone}</Label>
          </div>
        ))}
      </div>
    </Section>
  )
}

// -----------------------------------------------------------------------
// 2. ProgressBar
// -----------------------------------------------------------------------

function ProgressBar({
  value,
  indeterminate,
  rate,
}: {
  value?: number
  indeterminate?: boolean
  rate?: string
}) {
  const pct =
    value === undefined ? 0 : Math.max(0, Math.min(100, Math.round(value)))
  return (
    <div className="flex w-full items-center gap-3">
      <div className="bg-foreground/8 relative h-1.5 flex-1 overflow-hidden rounded-full">
        {indeterminate ?
          <div className="bg-brand animate-progress-indeterminate absolute inset-y-0 w-1/3 rounded-full" />
        : <div
            className="bg-brand h-full rounded-full transition-[width] duration-200"
            style={{ width: `${pct}%` }}
          />
        }
      </div>
      {rate ?
        <span className="text-foreground-alt/70 font-mono text-[0.65rem] tabular-nums">
          {rate}
        </span>
      : !indeterminate ?
        <span className="text-foreground-alt/70 w-8 text-right font-mono text-[0.65rem] tabular-nums">
          {pct}%
        </span>
      : null}
    </div>
  )
}

function ProgressBarSection() {
  // Animate a demo determinate bar so motion is visible.
  const [value, setValue] = useState(12)
  useEffect(() => {
    const id = setInterval(() => {
      setValue((v) => (v >= 100 ? 0 : v + 3))
    }, 300)
    return () => clearInterval(id)
  }, [])

  return (
    <Section
      title="ProgressBar"
      description="Determinate (with percent or rate label) and indeterminate variants. Uses bg-brand (fixes today's bg-primary drift). Height h-1.5; full-width; tabular-nums labels."
    >
      <div className="space-y-4">
        <div>
          <Label>Determinate (0 - 100, animated)</Label>
          <div className="mt-1.5">
            <ProgressBar value={value} />
          </div>
        </div>
        <div>
          <Label>Determinate + rate label</Label>
          <div className="mt-1.5">
            <ProgressBar value={62} rate="1.5 MiB/s" />
          </div>
        </div>
        <div>
          <Label>Indeterminate</Label>
          <div className="mt-1.5">
            <ProgressBar indeterminate />
          </div>
        </div>
        <div>
          <Label>Indeterminate + rate label</Label>
          <div className="mt-1.5">
            <ProgressBar indeterminate rate="Uploading" />
          </div>
        </div>
        <div className="grid grid-cols-4 gap-3">
          {[10, 35, 65, 92].map((v) => (
            <div key={v}>
              <ProgressBar value={v} />
            </div>
          ))}
        </div>
      </div>
    </Section>
  )
}

// -----------------------------------------------------------------------
// 3. LoadingCard
// -----------------------------------------------------------------------

type LoadingState = 'loading' | 'active' | 'synced' | 'error'

interface LoadingView {
  state: LoadingState
  title: string
  detail?: string
  progress?: number
  rate?: { up?: string; down?: string }
  lastActivity?: string
  error?: string
  onRetry?: () => void
  onCancel?: () => void
}

function LoadingCard({ view }: { view: LoadingView }) {
  const { state } = view
  return (
    <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5 backdrop-blur-sm">
      <div className="flex items-start gap-3">
        <div
          className={cn(
            'flex h-8 w-8 shrink-0 items-center justify-center rounded-md',
            state === 'loading' && 'bg-foreground/5 text-foreground-alt',
            state === 'active' && 'bg-brand/10 text-brand',
            state === 'synced' && 'bg-foreground/5 text-brand',
            state === 'error' && 'bg-destructive/10 text-destructive',
          )}
        >
          <LoadingCardIcon state={state} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-foreground text-sm font-semibold tracking-tight">
            {view.title}
          </div>
          {view.detail ?
            <div className="text-foreground-alt/60 mt-0.5 text-xs leading-relaxed">
              {view.detail}
            </div>
          : null}
          {view.progress !== undefined ?
            <div className="mt-2.5">
              <ProgressBar
                value={view.progress * 100}
                rate={view.rate?.down ?? view.rate?.up}
              />
            </div>
          : null}
          {view.rate && view.progress === undefined ?
            <div className="mt-2 grid grid-cols-2 gap-2">
              <RatePill label="Up" value={view.rate.up ?? '0 B/s'} />
              <RatePill label="Down" value={view.rate.down ?? '0 B/s'} />
            </div>
          : null}
          {view.lastActivity ?
            <div className="text-foreground-alt/40 mt-2 text-[0.65rem]">
              {view.lastActivity}
            </div>
          : null}
          {view.error ?
            <div className="bg-destructive/5 border-destructive/15 text-destructive mt-2 rounded-md border px-2 py-1 text-[0.65rem] leading-relaxed">
              {view.error}
            </div>
          : null}
          {view.onRetry || view.onCancel ?
            <div className="mt-2.5 flex gap-2">
              {view.onRetry ?
                <button
                  type="button"
                  onClick={view.onRetry}
                  className="border-foreground/8 bg-foreground/5 hover:bg-foreground/10 hover:border-foreground/15 text-foreground-alt hover:text-foreground rounded-md border px-2 py-1 text-[0.65rem] font-medium transition-all duration-150"
                >
                  Retry
                </button>
              : null}
              {view.onCancel ?
                <button
                  type="button"
                  onClick={view.onCancel}
                  className="text-foreground-alt/60 hover:text-foreground-alt rounded-md px-2 py-1 text-[0.65rem] font-medium transition-colors"
                >
                  Cancel
                </button>
              : null}
            </div>
          : null}
        </div>
      </div>
    </div>
  )
}

function LoadingCardIcon({ state }: { state: LoadingState }) {
  if (state === 'error') {
    return <LuCircleAlert className="h-4 w-4" aria-hidden="true" />
  }
  if (state === 'synced') {
    return (
      <span
        className="relative flex h-4 w-4 items-center justify-center"
        aria-hidden="true"
      >
        <LuCloud className="h-3.5 w-3.5" />
        <LuCircleCheck className="text-brand absolute -right-1 -bottom-1 h-2.5 w-2.5" />
      </span>
    )
  }
  return <Spinner size="md" />
}

function RatePill({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-foreground/6 bg-foreground/5 rounded-md border px-2 py-1">
      <div className="text-foreground-alt/50 text-[0.55rem] font-medium tracking-widest uppercase">
        {label}
      </div>
      <div className="text-foreground text-xs font-semibold tabular-nums">
        {value}
      </div>
    </div>
  )
}

function LoadingCardSection() {
  const examples: Array<{ label: string; view: LoadingView }> = [
    {
      label: 'loading (awaiting first status)',
      view: {
        state: 'loading',
        title: 'Checking sync status',
        detail: 'Waiting for the session sync watcher.',
      },
    },
    {
      label: 'active (working, with rate pills)',
      view: {
        state: 'active',
        title: 'Uploading changes',
        detail: 'Sending recent edits to the cloud.',
        rate: { up: '1.5 MiB/s', down: '24 KiB/s' },
        lastActivity: 'Last activity 10:42 PM',
      },
    },
    {
      label: 'active + determinate progress',
      view: {
        state: 'active',
        title: 'Cloning repository',
        detail: 'Receiving objects from origin.',
        progress: 0.62,
        rate: { down: '4.8 MiB/s' },
      },
    },
    {
      label: 'active + indeterminate (object load)',
      view: {
        state: 'active',
        title: 'Loading space',
        detail: 'Resolving root, path, stat, readdir (4 stages).',
      },
    },
    {
      label: 'synced (idle, caught up)',
      view: {
        state: 'synced',
        title: 'Synced',
        detail: 'All sync work complete.',
        rate: { up: '0 B/s', down: '0 B/s' },
        lastActivity: 'Last activity 5 min ago',
      },
    },
    {
      label: 'error (transport down)',
      view: {
        state: 'error',
        title: 'Sync needs attention',
        detail: 'Reconnect to finish uploading changes.',
        error: 'Transport error: WebSocket closed (code 1006).',
        onRetry: () => {},
        onCancel: () => {},
      },
    },
  ]

  return (
    <Section
      title="LoadingCard"
      description="The primary loading surface. Glass card with h-8 w-8 icon box + title + detail. Four states. Optionally renders a ProgressBar, rate pills, last-activity footer, error box, and retry/cancel buttons. Lifts the shape from SessionSyncStatusSummary."
    >
      <div className="space-y-3">
        {examples.map(({ label, view }) => (
          <div key={label}>
            <Label>{label}</Label>
            <div className="mt-1.5">
              <LoadingCard view={view} />
            </div>
          </div>
        ))}
      </div>
    </Section>
  )
}

// -----------------------------------------------------------------------
// 4. LoadingInline
// -----------------------------------------------------------------------

function LoadingInline({
  label,
  size = 'sm',
  tone = 'muted',
}: {
  label: string
  size?: SpinnerSize
  tone?: 'brand' | 'muted' | 'destructive'
}) {
  const toneCls =
    tone === 'brand' ? 'text-brand'
    : tone === 'destructive' ? 'text-destructive'
    : 'text-foreground-alt'
  return (
    <span className={cn('inline-flex items-center gap-1.5', toneCls)}>
      <Spinner size={size} />
      <span className="text-xs">{label}</span>
    </span>
  )
}

function LoadingInlineSection() {
  return (
    <Section
      title="LoadingInline"
      description="Spinner + one-line label. Replaces ~15 ad-hoc <LuLoader/> + text sites (button labels, row pending states, single-line loaders)."
    >
      <div className="space-y-3">
        <div>
          <Label>In place of bare text</Label>
          <div className="border-foreground/6 bg-background-card/30 mt-1.5 rounded-lg border p-3 backdrop-blur-sm">
            <LoadingInline label="Loading messages..." />
          </div>
        </div>
        <div>
          <Label>As a button label (pending action)</Label>
          <div className="mt-1.5 flex gap-2">
            <button
              type="button"
              className="border-foreground/8 bg-foreground/5 text-foreground flex items-center gap-2 rounded-md border px-3 py-1.5 text-xs"
              disabled
            >
              <Spinner size="sm" className="text-brand" />
              <span>Creating...</span>
            </button>
            <button
              type="button"
              className="border-destructive/15 bg-destructive/5 text-destructive flex items-center gap-2 rounded-md border px-3 py-1.5 text-xs"
              disabled
            >
              <Spinner size="sm" />
              <span>Retrying...</span>
            </button>
          </div>
        </div>
        <div>
          <Label>Tones</Label>
          <div className="mt-1.5 flex gap-4">
            <LoadingInline label="Muted" tone="muted" />
            <LoadingInline label="Brand" tone="brand" />
            <LoadingInline label="Destructive" tone="destructive" />
          </div>
        </div>
      </div>
    </Section>
  )
}

// -----------------------------------------------------------------------
// 5. LoadingScreen (mini preview)
// -----------------------------------------------------------------------

function LoadingScreenSection() {
  const [progress, setProgress] = useState(0)
  useEffect(() => {
    const id = setInterval(() => {
      setProgress((p) => (p >= 100 ? 0 : p + 2))
    }, 200)
    return () => clearInterval(id)
  }, [])

  return (
    <Section
      title="LoadingScreen (mini preview)"
      description="Full-screen boot state. The real component keeps its animated logo + shine border; only the title / detail / progress treatment gets refreshed to match the other primitives. Mini-preview here to avoid taking over the viewport."
    >
      <div className="border-foreground/6 bg-background/80 relative flex h-64 items-center justify-center overflow-hidden rounded-lg border">
        <div className="relative z-10 flex flex-col items-center gap-4">
          <div className="bg-brand/10 flex h-12 w-12 items-center justify-center rounded-xl">
            <Spinner size="xl" className="text-brand" />
          </div>
          <div className="space-y-1 text-center">
            <div className="text-foreground text-lg font-semibold tracking-tight select-none">
              Loading Spacewave
            </div>
            <div className="text-foreground-alt/60 text-xs select-none">
              Setting up your session...
            </div>
          </div>
          <div className="w-56">
            <ProgressBar value={progress} />
          </div>
        </div>
      </div>
    </Section>
  )
}
