import { LuSiren } from 'react-icons/lu'

import type { Alarm, AlarmOption } from '@s4wave/sdk/alarm/alarm.pb.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'

import { alarmStatus, holdText, type AlarmTone } from './alarm-view.js'

export interface AlarmViewProps {
  alarm: Alarm
  nowMs: number
  // who names the member of a peer ID.
  who: (peerId: string) => string
  // canAcknowledge is set for a writer.
  canAcknowledge: boolean
  saving: boolean
  // ackError is why the last acknowledgement failed.
  ackError: string | null
  onAcknowledge: () => void
}

// AlarmView shows an Alarm: its state, the condition, the actions that would
// change it, and a writer's Acknowledge.
export function AlarmView({
  alarm,
  nowMs,
  who,
  canAcknowledge,
  saving,
  ackError,
  onAcknowledge,
}: AlarmViewProps) {
  const status = alarmStatus(alarm, nowMs, who)
  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <header className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <LuSiren
          className={cn(
            'size-3.5',
            status.tone === 'alert'
              ? 'text-destructive'
              : 'text-foreground-alt',
          )}
          aria-hidden
        />
        <span
          className={cn(
            'text-sm font-semibold tracking-tight',
            status.tone === 'alert' ? 'text-destructive' : 'text-foreground',
          )}
        >
          {alarm.title || 'Alarm'}
        </span>
        <AlarmBadge word={status.word} tone={status.tone} />
      </header>

      <div className="min-h-0 flex-1 space-y-4 overflow-auto p-4">
        <div className="space-y-1">
          <p className="text-foreground text-sm">{status.line}</p>
          {alarm.summary ? (
            <p className="text-foreground-alt text-sm">{alarm.summary}</p>
          ) : null}
          {!status.sounding ? (
            <p className="text-foreground-alt text-xs">{holdText(alarm)}</p>
          ) : null}
        </div>

        {alarm.error ? (
          <p className="text-warning text-xs">
            The watch could not check this Alarm:{' '}
            {alarm.error.replace(/\.$/, '')}.
          </p>
        ) : null}

        {canAcknowledge || ackError ? (
          <AckRow
            firing={status.word === 'Firing'}
            canAcknowledge={canAcknowledge}
            saving={saving}
            ackError={ackError}
            onAcknowledge={onAcknowledge}
          />
        ) : null}

        {status.sounding ? (
          <AlarmOptions options={alarm.options ?? []} />
        ) : null}
      </div>
    </div>
  )
}

// AckRow is a writer's Acknowledge while the Alarm fires, with why the last
// acknowledgement failed.
function AckRow({
  firing,
  canAcknowledge,
  saving,
  ackError,
  onAcknowledge,
}: Pick<
  AlarmViewProps,
  'canAcknowledge' | 'saving' | 'ackError' | 'onAcknowledge'
> & { firing: boolean }) {
  if (!firing) {
    return ackError ? (
      <p className="text-foreground-alt text-xs">{ackError}</p>
    ) : null
  }
  if (!canAcknowledge) return null
  return (
    <div className="flex items-center gap-3">
      <Button size="sm" disabled={saving} onClick={onAcknowledge}>
        {ackError ? 'Retry' : 'Acknowledge'}
      </Button>
      {ackError ? (
        <span className="text-destructive text-xs">{ackError}</span>
      ) : null}
    </div>
  )
}

// AlarmOptions lists the actions that would change the condition, the
// recommended one first and marked.
function AlarmOptions({ options }: { options: readonly AlarmOption[] }) {
  if (options.length === 0) return null
  const sorted = [...options].sort(
    (a, b) => Number(b.recommended ?? false) - Number(a.recommended ?? false),
  )
  return (
    <section className="max-w-xl space-y-1.5">
      <h3 className="text-foreground-alt text-xs font-medium">
        What would change this
      </h3>
      <ul className="space-y-1.5">
        {sorted.map((o) => (
          <li
            key={o.label}
            className={cn(
              'rounded border px-3 py-2 text-sm',
              o.recommended
                ? 'border-foreground/20 bg-foreground/5'
                : 'border-foreground/8',
            )}
          >
            <div className="text-foreground flex items-center gap-2">
              {o.label}
              {o.recommended ? (
                <span className="text-metadata text-foreground-alt">
                  Recommended
                </span>
              ) : null}
            </div>
            {o.description ? (
              <div className="text-foreground-alt text-xs">{o.description}</div>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  )
}

// AlarmBadge labels an Alarm's state with its word and a status color.
export function AlarmBadge({
  word,
  tone,
  className,
}: {
  word: string
  tone: AlarmTone
  className?: string
}) {
  return (
    <span
      // text-metadata stays outside cn, which reads it as a color.
      className={`text-metadata ${cn(
        'rounded px-1.5 py-px font-medium',
        tone === 'alert' && 'bg-destructive/15 text-destructive',
        tone === 'holding' && 'bg-warning/15 text-warning',
        tone === 'quiet' && 'bg-foreground/8 text-foreground-alt',
        className,
      )}`}
    >
      {word}
    </span>
  )
}
