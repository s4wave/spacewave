import { useState } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useAppQuery, useAppMutation } from '@s4wave/web/sync/app-hooks.js'
import { useAppAttachment } from '@s4wave/web/sync/useAppAttachment.js'
import { cn } from '@s4wave/web/style/utils.js'
import { colors, rankedColors } from './app.js'

/** ColorViewer keeps selection and search local while votes synchronize through World. */
export default function ColorViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  // The viewer owns its attachment; query and mutation hooks share its authority.
  const app = useAppAttachment(worldState, colors, getObjectKey(objectInfo))
  const [selected, setSelected] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const result = useAppQuery(app.value, rankedColors, { search })
  const vote = useAppMutation(app.value, 'like')
  const initializing = useAppMutation(app.value, 'initialize')

  // Failed attachments and queries stay visible with a concrete recovery action.
  if (app.error || result.status === 'error') {
    const error = app.error ?? (result.status === 'error' ? result.error : null)
    return (
      <div role="alert" className="p-4">
        <p>{error?.message}</p>
        <button type="button" onClick={app.retry}>
          Reopen color votes
        </button>
      </div>
    )
  }
  if (app.loading || result.status !== 'current') {
    return (
      <p role="status" className="p-4">
        Loading colors…
      </p>
    )
  }
  const current = result.value.find(({ key }) => key === selected)
  const pending = vote.state.status === 'pending'

  // Shared values are rendered directly from the query; no optimistic copy is kept.
  return (
    <section className="mx-auto flex max-w-xl flex-col gap-4 p-4">
      <header>
        <h2 className="text-xl font-semibold">Color votes</h2>
        <p className="text-muted-foreground text-sm">
          Pick a color. Every vote is shared with this Space.
        </p>
      </header>
      <label className="flex flex-col gap-1 text-sm">
        Find a color
        <input
          className="border-border rounded border p-2"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
      </label>
      <ul className="flex flex-col gap-2" aria-label="Colors ranked by votes">
        {result.value.map(({ key, value }) => (
          <li key={key}>
            <button
              type="button"
              aria-pressed={selected === key}
              onClick={() => setSelected(key)}
              className={cn(
                'border-border flex w-full items-center gap-3 rounded border p-3 text-left',
                selected === key && 'bg-background-secondary',
              )}
            >
              <svg aria-hidden="true" className="size-6" viewBox="0 0 24 24">
                <circle cx="12" cy="12" r="12" fill={value.hex} />
              </svg>
              <span className="flex-1">{value.name}</span>
              <span>
                {value.likes} {value.likes === 1 ? 'vote' : 'votes'}
              </span>
            </button>
          </li>
        ))}
      </ul>
      {result.value.length === 0 && (
        <p>
          {search ? 'No matching colors.' : 'Add the starter colors to begin.'}
        </p>
      )}
      {!search && result.value.length === 0 && (
        <button
          type="button"
          disabled={initializing.state.status === 'pending'}
          onClick={() => initializing.submit(null)}
        >
          Add starter colors
        </button>
      )}
      <button
        type="button"
        disabled={!current || pending}
        className="bg-primary text-primary-foreground rounded px-4 py-2 disabled:opacity-50"
        onClick={() => current && vote.submit({ id: current.key })}
      >
        {pending
          ? 'Saving vote…'
          : current
            ? `Vote for ${current.value.name}`
            : 'Select a color to vote'}
      </button>
      {vote.state.status === 'error' && (
        <div role="alert">
          <p>{vote.state.error.message}</p>
          <button type="button" onClick={vote.retry}>
            Retry vote
          </button>
        </div>
      )}
      {initializing.state.status === 'error' && (
        <div role="alert">
          <p>{initializing.state.error.message}</p>
          <button type="button" onClick={initializing.retry}>
            Retry starter colors
          </button>
        </div>
      )}
    </section>
  )
}
