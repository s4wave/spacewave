import type { ReactNode } from 'react'
import { LuActivity } from 'react-icons/lu'

import type {
  Label,
  TargetInfo,
  TargetStatus,
} from '@s4wave/sdk/prom/prom.pb.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { FactList, FactRow } from '@s4wave/web/ui/FactList.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'

import { formatAgo, formatClock, targetName } from './prom-view.js'
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
      <ViewerBar
        icon={<LuActivity />}
        title={targetName(objectKey)}
        state={<StateBadge state={state} />}
      />

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

        <FactList className="max-w-md">
          {scrape?.url ? (
            <FactRow label="URL" value={scrape.url} />
          ) : (
            <FactRow
              label="URL"
              value="None: stores only samples written to it"
              kind="text"
              control={null}
            />
          )}
          <FactRow
            label="Every"
            value={`${intervalMs / 1000} s`}
            kind="text"
            control={null}
          />
          <TargetLabels labels={info?.meta?.labels?.labels ?? []} />
          <FactRow
            label="Keeps"
            value={`${Math.round(retentionMs / 86_400_000)} days`}
            kind="text"
            control={null}
          />
        </FactList>
      </div>
    </div>
  )
}

// TargetLabels lists a target's labels one per row, the name as the row's
// label and the value copyable, so a long set never hides a label behind a
// cut. A target with no labels shows None.
function TargetLabels({ labels }: { labels: readonly Label[] }) {
  if (labels.length === 0) {
    return <FactRow label="Labels" value="None" kind="text" control={null} />
  }
  return labels.map((l) => (
    <FactRow
      key={l.name}
      label={l.name ?? ''}
      value={l.value ?? ''}
      kind="text"
    />
  ))
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
    <FactList className="max-w-md">
      <FactRow
        label="Device"
        value={deviceName(status.devicePeerId ?? '')}
        kind="text"
        control={null}
      />
      <FactRow
        label="Last scrape"
        value={formatClock(Number(status.lastScrapeMs ?? 0n))}
        kind="text"
        control={null}
      />
      <FactRow
        label="Duration"
        value={`${status.lastDurationMs ?? 0} ms`}
        kind="text"
        control={null}
      />
      <FactRow
        label="Series"
        value={String(status.series ?? 0n)}
        kind="text"
        control={null}
      />
    </FactList>
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
