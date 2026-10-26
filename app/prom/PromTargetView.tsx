import type { ReactNode } from 'react'
import { LuActivity } from 'react-icons/lu'

import type { TargetInfo, TargetStatus } from '@s4wave/sdk/prom/prom.pb.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'

import {
  formatAgo,
  formatClock,
  formatLabels,
  targetName,
} from './prom-view.js'
import type { TargetAction, TargetState } from './target-state.js'

// actionLabels are the approval buttons' labels.
const actionLabels: Record<Exclude<TargetAction, null>, string> = {
  'scrape-here': 'Scrape from this Device',
  'take-over': 'Scrape from this Device instead',
  stop: 'Stop scraping',
}

export interface PromTargetViewProps {
  objectKey: string
  info: TargetInfo | undefined
  state: TargetState
  action: TargetAction
  // deviceName names the Device of a peer ID.
  deviceName: (peerId: string) => string
  nowMs: number
  saving: boolean
  // saveError is the reason the last approval write failed.
  saveError: string | null
  onAction: () => void
  // chart shows up and scrape_duration_seconds over the time range.
  chart?: ReactNode
}

// PromTargetView shows a Scrape target's configuration, its scrape state,
// and the approval action this member can take on this Device.
export function PromTargetView({
  objectKey,
  info,
  state,
  action,
  deviceName,
  nowMs,
  saving,
  saveError,
  onAction,
  chart,
}: PromTargetViewProps) {
  const scrape = info?.meta?.scrape
  const intervalMs = scrape?.intervalMs || 15_000
  const retentionMs = Number(info?.meta?.retentionMs ?? 0n) || 15 * 86_400_000
  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <header className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <LuActivity className="text-foreground-alt size-3.5" aria-hidden />
        <span className="text-foreground text-sm font-semibold tracking-tight">
          {targetName(objectKey)}
        </span>
        <StateBadge state={state} />
      </header>

      <div className="min-h-0 flex-1 space-y-4 overflow-auto p-4">
        <p className="text-foreground text-sm">
          {describeState(state, nowMs, deviceName)}
        </p>

        {action ? (
          <ActionRow
            action={action}
            saving={saving}
            saveError={saveError}
            onAction={onAction}
          />
        ) : null}

        {state.state === 'up' || state.state === 'down' ? (
          <ScrapeFields status={state.status} deviceName={deviceName} />
        ) : null}

        {chart}

        <dl className="grid max-w-md grid-cols-2 gap-x-6 gap-y-1 text-xs">
          <Field label="URL" mono>
            {scrape?.url || 'none: stores only samples written to it'}
          </Field>
          <Field label="Every">{intervalMs / 1000} s</Field>
          <Field label="Labels" mono>
            {formatLabels(info?.meta?.labels)}
          </Field>
          <Field label="Keeps">
            {Math.round(retentionMs / 86_400_000)} days
          </Field>
        </dl>
      </div>
    </div>
  )
}

// describeState writes the sentence for the target's state.
function describeState(
  state: TargetState,
  nowMs: number,
  deviceName: (peerId: string) => string,
): string {
  switch (state.state) {
    case 'unapproved':
      return 'No Device scrapes this target yet.'
    case 'waiting':
      return 'Waiting for the first scrape from this Device.'
    case 'up':
      return `Up. Last scraped ${formatAgo(Number(state.status.lastScrapeMs ?? 0n), nowMs)} ago.`
    case 'down':
      return `Down: ${state.status.lastError || 'the scrape failed'}.`
    case 'stale':
      return `No scrape since ${formatClock(Number(state.status.lastScrapeMs ?? 0n))}. ${deviceName(state.status.devicePeerId ?? '')} may be offline.`
  }
}

// ActionRow is the approval button and the last write's failure.
function ActionRow({
  action,
  saving,
  saveError,
  onAction,
}: {
  action: Exclude<TargetAction, null>
  saving: boolean
  saveError: string | null
  onAction: () => void
}) {
  return (
    <div className="flex items-center gap-3">
      <Button
        size="sm"
        variant={action === 'stop' ? 'outline' : 'default'}
        disabled={saving}
        onClick={onAction}
      >
        {saveError ? 'Retry' : actionLabels[action]}
      </Button>
      {saveError ? (
        <span className="text-destructive text-xs">
          Could not save: {saveError}.
        </span>
      ) : null}
    </div>
  )
}

// ScrapeFields lists the last scrape's Device, time, duration and series.
function ScrapeFields({
  status,
  deviceName,
}: {
  status: TargetStatus
  deviceName: (peerId: string) => string
}) {
  return (
    <dl className="grid max-w-md grid-cols-2 gap-x-6 gap-y-1 text-xs">
      <Field label="Device">{deviceName(status.devicePeerId ?? '')}</Field>
      <Field label="Last scrape">
        {formatClock(Number(status.lastScrapeMs ?? 0n))}
      </Field>
      <Field label="Duration">{status.lastDurationMs ?? 0} ms</Field>
      <Field label="Series">{String(status.series ?? 0n)}</Field>
    </dl>
  )
}

// StateBadge labels the state with an icon-free word and a status color.
function StateBadge({ state }: { state: TargetState }) {
  const labels: Record<TargetState['state'], string> = {
    unapproved: 'Unapproved',
    waiting: 'Waiting',
    up: 'Up',
    down: 'Down',
    stale: 'Stale',
  }
  return (
    <span
      // text-metadata stays outside cn, which reads it as a color.
      className={`text-metadata ${cn(
        'rounded px-1.5 py-px font-medium',
        state.state === 'up' && 'bg-success/15 text-success',
        state.state === 'down' && 'bg-destructive/15 text-destructive',
        state.state === 'stale' && 'bg-warning/15 text-warning',
        (state.state === 'unapproved' || state.state === 'waiting') &&
          'bg-foreground/8 text-foreground-alt',
      )}`}
    >
      {labels[state.state]}
    </span>
  )
}

// Field is one term and its value.
function Field({
  label,
  mono,
  children,
}: {
  label: string
  mono?: boolean
  children: ReactNode
}) {
  return (
    <>
      <dt className="text-foreground-alt">{label}</dt>
      <dd className={cn('text-foreground break-all', mono && 'font-mono')}>
        {children}
      </dd>
    </>
  )
}
