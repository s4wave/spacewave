import { useCallback, useReducer } from 'react'

import type { PromDashboard } from '@s4wave/sdk/prom/prom.js'
import type { TargetMeta } from '@s4wave/sdk/prom/prom.pb.js'

import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    marginInline: 'auto',
    display: 'grid',
    width: '100%',
    maxWidth: 'var(--container-md)',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    display: 'flex',
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule4: { flex: 1 },
  rule5: { width: `calc(${themeLayout['--spacing']} * 28)` },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
})

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
  // onCreated receives the new target's object key.
  onCreated: (objectKey: string) => void
  onCancel: () => void
}

// PromTargetForm creates a Scrape target with its name, exporter URL, job
// label and interval. The name becomes the target's key and instance label.
export function PromTargetForm({
  dashboard,
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

  return (
    <form
      aria-label="Add Scrape target"
      sx={stylexStyles.rule1}
      onSubmit={(e) => {
        e.preventDefault()
        if (valid) void create()
      }}
    >
      <label sx={stylexStyles.rule2}>
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
      <label sx={stylexStyles.rule2}>
        Exporter URL
        <Input
          variant="wizardMono"
          value={form.url}
          onChange={(e) =>
            dispatch({ kind: 'edit', patch: { url: e.target.value } })
          }
        />
      </label>
      <div sx={stylexStyles.rule3}>
        <label sx={[stylexStyles.rule2, stylexStyles.rule4]}>
          Job
          <Input
            variant="wizard"
            value={form.job}
            onChange={(e) =>
              dispatch({ kind: 'edit', patch: { job: e.target.value } })
            }
          />
        </label>
        <label sx={[stylexStyles.rule2, stylexStyles.rule5]}>
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
        <p sx={stylexStyles.rule6}>Could not save: {form.save.reason}.</p>
      ) : null}
      <div sx={stylexStyles.rule3}>
        <Button
          type="submit"
          size="sm"
          disabled={!valid || form.save.state === 'saving'}
        >
          {form.save.state === 'failed' ? 'Retry' : 'Add Scrape target'}
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  )
}
