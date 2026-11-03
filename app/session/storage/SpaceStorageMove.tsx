import { useCallback, useEffect, useId, useRef, useState } from 'react'
import { LuArrowRightLeft } from '@s4wave/web/images/icons.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import {
  MoveSpaceStoragePhase,
  type StorageBackendInfo,
} from '@s4wave/sdk/session/session.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import {
  accountStorageChoice,
  describeMoveProgress,
} from './storage-backend.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    minWidth: 0,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

interface SpaceStorageMoveProps {
  sharedObjectId: string
  // placedId is the backend holding the Space, empty for the account's own storage.
  placedId: string
  backends: StorageBackendInfo[]
}

// SpaceStorageMove moves a Space to another storage backend and shows the
// progress. Leaving the page stops the progress stream; the upload continues.
export function SpaceStorageMove({
  sharedObjectId,
  placedId,
  backends,
}: SpaceStorageMoveProps) {
  const id = useId()
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const [target, setTarget] = useState('')
  const [progress, setProgress] = useState('')
  const [moving, setMoving] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  useEffect(() => () => abortRef.current?.abort(), [])

  const destinations = [
    ...(placedId
      ? [{ id: accountStorageChoice, name: "the account's own storage" }]
      : []),
    ...backends
      .filter((b) => b.backend?.id && b.backend.id !== placedId)
      .map((b) => ({
        id: b.backend?.id ?? '',
        name: b.backend?.displayName ?? '',
      })),
  ]
  const selected = destinations.find((d) => d.id === target) ?? destinations[0]

  const handleMove = useCallback(async () => {
    if (!session || !selected) return
    const abort = new AbortController()
    abortRef.current = abort
    const backendId = selected.id === accountStorageChoice ? '' : selected.id
    setMoving(true)
    try {
      for await (const step of session.moveSpaceStorage(
        sharedObjectId,
        backendId,
        abort.signal,
      )) {
        setProgress(describeMoveProgress(step, selected.name))
        if (step.phase === MoveSpaceStoragePhase.MoveSpaceStoragePhase_DONE) {
          toast.success(`Moved to ${selected.name}`)
        }
      }
    } catch (err) {
      if (!abort.signal.aborted) {
        toast.error(err instanceof Error ? err.message : String(err))
      }
    } finally {
      setMoving(false)
      setProgress('')
    }
  }, [session, selected, sharedObjectId])

  if (!selected) {
    return null
  }
  if (moving) {
    return (
      <p sx={stylexStyles2.rule1} data-testid="space-storage-move-progress">
        {progress || `Moving to ${selected.name}`}
      </p>
    )
  }
  return (
    <div sx={stylexStyles2.rule2}>
      <label htmlFor={id} sx={stylexStyles2.rule3}>
        Move to
      </label>
      <select
        id={id}
        value={selected.id}
        onChange={(e) => setTarget(e.target.value)}
        sx={stylexStyles2.rule4}
        data-testid="space-storage-move-target"
      >
        {destinations.map((d) => (
          <option key={d.id} value={d.id}>
            {d.name}
          </option>
        ))}
      </select>
      <DashboardButton
        icon={<LuArrowRightLeft sx={stylexStyles.rule1} />}
        onClick={() => void handleMove()}
      >
        Move
      </DashboardButton>
    </div>
  )
}
