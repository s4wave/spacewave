import { useState } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useAppQuery, useAppMutation } from '@s4wave/web/sync/app-hooks.js'
import { useAppAttachment } from '@s4wave/web/sync/useAppAttachment.js'

import { colors, rankedColors } from './app.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { padding: `calc(${themeLayout['--spacing']} * 4)` },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: themeColors['--color-border'],
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    textAlign: 'left',
    width: '100%',
  },
  rule4: { backgroundColor: themeColors['--color-background-secondary'] },
  rule5: {
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule6: { flex: 1 },
  rule7: {
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: themeColors['--color-primary'],
    color: themeColors['--color-primary-foreground'],
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule8: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    marginInline: 'auto',
    display: 'flex',
    flexDirection: 'column',
    maxWidth: 'var(--container-xl)',
  },
  rule9: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
  },
  rule10: {
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule11: {
    gap: themeLayout['--spacing'],
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule12: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    borderColor: themeColors['--color-border'],
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
  },
})

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
    <div role="alert" sx={stylexStyles.rule1}>
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
    <ul sx={stylexStyles.rule2} aria-label="Colors ranked by votes">
      {colors.map(({ key, value }) => (
        <li key={key}>
          <button
            type="button"
            aria-pressed={selected === key}
            onClick={() => onSelect(key)}
            sx={[stylexStyles.rule3, selected === key && stylexStyles.rule4]}
          >
            <svg aria-hidden="true" sx={stylexStyles.rule5} viewBox="0 0 24 24">
              <circle cx="12" cy="12" r="12" fill={value.hex} />
            </svg>
            <span sx={stylexStyles.rule6}>{value.name}</span>
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
      sx={stylexStyles.rule7}
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
      <p role="status" sx={stylexStyles.rule1}>
        Loading colors…
      </p>
    )
  }
  const current = result.value.find(({ key }) => key === selected)

  // Shared values are rendered directly from the query; no optimistic copy is kept.
  return (
    <section sx={stylexStyles.rule8}>
      <header>
        <h2 sx={stylexStyles.rule9}>Color votes</h2>
        <p sx={stylexStyles.rule10}>
          Pick a color. Every vote is shared with this Space.
        </p>
      </header>
      <label sx={stylexStyles.rule11}>
        Find a color
        <input
          sx={stylexStyles.rule12}
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
