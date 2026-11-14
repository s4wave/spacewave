import { useCallback, useReducer } from 'react'

import type { PromDashboard } from '@s4wave/sdk/prom/prom.js'
import type { TargetMeta } from '@s4wave/sdk/prom/prom.pb.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'

// targetKeyPrefix is the World key prefix of Scrape targets.
const targetKeyPrefix = 'prom/target/'

// Form is the state of the new target form.
type Form = {
  name: string
  url: string
  job: string
  intervalSec: string
  save:
    | { state: 'idle' }
    | { state: 'saving' }
    | { state: 'failed'; reason: string }
}

// FormEvent moves the form between states.
type FormEvent =
  | { kind: 'edit'; patch: Partial<Omit<Form, 'save'>> }
  | { kind: 'saving' }
  | { kind: 'failed'; reason: string }

// reduce applies one event to the form.
function reduce(form: Form, ev: FormEvent): Form {
  switch (ev.kind) {
    case 'edit':
      return { ...form, ...ev.patch }
    case 'saving':
      return { ...form, save: { state: 'saving' } }
    case 'failed':
      return { ...form, save: { state: 'failed', reason: ev.reason } }
  }
}

const initialForm: Form = {
  name: '',
  url: 'http://127.0.0.1:9100/metrics',
  job: 'node',
  intervalSec: '15',
  save: { state: 'idle' },
}

export interface PromTargetFormProps {
  dashboard: PromDashboard
  // canSave is unset while the Dashboard's handle is lost: the form keeps
  // its fields and offers no submit until the next handle.
  canSave: boolean
  // onCreated receives the new target's object key.
  onCreated: (objectKey: string) => void
  onCancel: () => void
}

// PromTargetForm creates a Scrape target with its name, exporter URL, job
// label and interval. The name becomes the target's key and instance label.
export function PromTargetForm({
  dashboard,
  canSave,
  onCreated,
  onCancel,
}: PromTargetFormProps) {
  const [form, dispatch] = useReducer(reduce, initialForm)
  const name = form.name.trim()
  const interval = Number(form.intervalSec)
  const valid =
    /^[a-z0-9][a-z0-9-]*$/.test(name) &&
    /^(https?:\/\/.+|device:metrics)$/.test(form.url.trim()) &&
    form.job.trim() !== '' &&
    interval >= 1

  // Create the target, then hand its key to the caller.
  const create = useCallback(async () => {
    dispatch({ kind: 'saving' })
    const objectKey = targetKeyPrefix + name
    const meta: TargetMeta = {
      labels: {
        labels: [
          { name: 'instance', value: name },
          { name: 'job', value: form.job.trim() },
        ],
      },
      scrape: { url: form.url.trim(), intervalMs: interval * 1000 },
    }
    try {
      await dashboard.createTarget(objectKey, meta)
      onCreated(objectKey)
    } catch (err) {
      dispatch({
        kind: 'failed',
        reason: err instanceof Error ? err.message : String(err),
      })
    }
  }, [dashboard, name, form.job, form.url, interval, onCreated])

  const label = 'text-foreground-alt flex flex-col gap-1 text-xs'
  return (
    <form
      aria-label="Add Scrape target"
      className="border-foreground/8 mx-auto grid w-full max-w-md gap-2 rounded-md border p-3"
      onSubmit={(e) => {
        e.preventDefault()
        if (valid && canSave) void create()
      }}
    >
      <label className={label}>
        Name
        <Input
          variant="wizard"
          placeholder="forge-node"
          value={form.name}
          onChange={(e) =>
            dispatch({ kind: 'edit', patch: { name: e.target.value } })
          }
        />
      </label>
      <label className={label}>
        Exporter URL
        <Input
          variant="wizardMono"
          value={form.url}
          onChange={(e) =>
            dispatch({ kind: 'edit', patch: { url: e.target.value } })
          }
        />
      </label>
      <div className="flex gap-2">
        <label className={cn(label, 'flex-1')}>
          Job
          <Input
            variant="wizard"
            value={form.job}
            onChange={(e) =>
              dispatch({ kind: 'edit', patch: { job: e.target.value } })
            }
          />
        </label>
        <label className={cn(label, 'w-28')}>
          Every (seconds)
          <Input
            variant="wizard"
            inputMode="numeric"
            value={form.intervalSec}
            onChange={(e) =>
              dispatch({ kind: 'edit', patch: { intervalSec: e.target.value } })
            }
          />
        </label>
      </div>
      {form.save.state === 'failed' ? (
        <p className="text-destructive text-xs">
          Could not save: {form.save.reason}.
        </p>
      ) : null}
      {!canSave ? (
        <p className="text-foreground-alt text-xs">
          Adding returns once the Dashboard is read again.
        </p>
      ) : null}
      <div className="flex gap-2">
        {canSave ? (
          <Button
            type="submit"
            size="sm"
            disabled={!valid || form.save.state === 'saving'}
          >
            {form.save.state === 'failed' ? 'Retry' : 'Add Scrape target'}
          </Button>
        ) : null}
        <Button type="button" variant="outline" size="sm" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  )
}
