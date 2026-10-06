import { useState } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useAppQuery, useAppMutation } from '@s4wave/web/sync/app-hooks.js'
import { useAppAttachment } from '@s4wave/web/sync/useAppAttachment.js'
import { cn } from '@s4wave/web/style/utils.js'
import { colors, rankedColors } from './app.js'

interface RankedColor {
  key: string
  value: { name: string; hex: string; likes: number }
}

// ColorViewerFailure shows a failed attachment or query with a recovery action.
function ColorViewerFailure({
  error,
  onRetry,
}: {
  error: Error | null
  onRetry: () => void
}) {
  return (
    <div role="alert" className="p-4">
      <p>{error?.message}</p>
      <button type="button" onClick={onRetry}>
        Reopen color votes
      </button>
    </div>
  )
}

// ColorChoices lists the ranked colors and reports the picked key.
function ColorChoices({
  colors,
  selected,
  onSelect,
}: {
  colors: RankedColor[]
  selected: string | null
  onSelect: (key: string) => void
}) {
  return (
    <ul className="flex flex-col gap-2" aria-label="Colors ranked by votes">
      {colors.map(({ key, value }) => (
        <li key={key}>
          <button
            type="button"
            aria-pressed={selected === key}
            onClick={() => onSelect(key)}
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
  )
}

// ColorVoteButton votes for the selected color while a vote is saving.
function ColorVoteButton({
  current,
  pending,
  onVote,
}: {
  current: RankedColor | undefined
  pending: boolean
  onVote: (key: string) => void
}) {
  let label = 'Select a color to vote'
  if (current) label = `Vote for ${current.value.name}`
  if (pending) label = 'Saving vote…'

  return (
    <button
      type="button"
      disabled={!current || pending}
      className="bg-primary text-primary-foreground rounded px-4 py-2 disabled:opacity-50"
      onClick={() => current && onVote(current.key)}
    >
      {label}
    </button>
  )
}

// MutationFailure shows a failed mutation with its retry action.
function MutationFailure({
  error,
  retryLabel,
  onRetry,
}: {
  error: Error
  retryLabel: string
  onRetry: () => void
}) {
  return (
    <div role="alert">
      <p>{error.message}</p>
      <button type="button" onClick={onRetry}>
        {retryLabel}
      </button>
    </div>
  )
}

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
    return <ColorViewerFailure error={error} onRetry={app.retry} />
  }
  if (app.loading || result.status !== 'current') {
    return (
      <p role="status" className="p-4">
        Loading colors…
      </p>
    )
  }
  const current = result.value.find(({ key }) => key === selected)

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
      <ColorChoices
        colors={result.value}
        selected={selected}
        onSelect={setSelected}
      />
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
      <ColorVoteButton
        current={current}
        pending={vote.state.status === 'pending'}
        onVote={(id) => vote.submit({ id })}
      />
      {vote.state.status === 'error' && (
        <MutationFailure
          error={vote.state.error}
          retryLabel="Retry vote"
          onRetry={vote.retry}
        />
      )}
      {initializing.state.status === 'error' && (
        <MutationFailure
          error={initializing.state.error}
          retryLabel="Retry starter colors"
          onRetry={initializing.retry}
        />
      )}
    </section>
  )
}
