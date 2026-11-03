import { useCallback, useEffect, useState } from 'react'
import {
  LuPlugZap,
  LuStar,
  LuStarOff,
  LuTrash2,
} from '@s4wave/web/images/icons.js'
import { isDesktop } from '@aptre/bldr'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { StorageBackendInfo } from '@s4wave/sdk/session/session.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { SpaceUploadStatus } from './SpaceUploadStatus.js'
import { StorageCheckResult } from './StorageCheckResult.js'
import {
  buildStorageCorsRule,
  describeStorageCheck,
  formatStorageLocation,
  guessStoragePreset,
  type StorageCheckView,
} from './storage-backend.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 2)` },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: { minWidth: 0 },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-brand'],
  },
  rule6: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule7: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule8: { marginTop: `calc(${themeLayout['--spacing']} * 2)` },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

interface StorageBackendRowProps {
  info: StorageBackendInfo
  isDefault: boolean
}

// StorageBackendRow shows one storage backend with its check result and
// stored size, the upload state of each Space placed on it, and its check,
// default, and remove actions.
export function StorageBackendRow({
  sx,
  info,
  isDefault,
}: StorageBackendRowProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const [check, setCheck] = useState<StorageCheckView | null>(null)
  const [busy, setBusy] = useState(false)
  const [confirmRemove, setConfirmRemove] = useState(false)

  const backend = info.backend
  const id = backend?.id ?? ''
  const name = backend?.displayName ?? ''
  const preset = guessStoragePreset(backend?.s3)
  const placed = info.placedSpaces ?? []

  // Run one action at a time. A failure is reported and returns undefined.
  const run = useCallback(async <T,>(action: () => Promise<T>) => {
    setBusy(true)
    try {
      return await action()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err))
      return undefined
    } finally {
      setBusy(false)
    }
  }, [])

  const handleCheck = useCallback(async () => {
    if (!session) return
    const resp = await run(() =>
      session.checkStorageBackend({ storageBackendId: id }),
    )
    if (resp) {
      setCheck(describeStorageCheck(resp.result, !isDesktop))
    }
  }, [id, run, session])

  // Check the bucket once the session is ready, so the row opens with its
  // health and stored size.
  useEffect(() => {
    void handleCheck()
  }, [handleCheck])

  const handleDefault = useCallback(() => {
    if (!session) return
    void run(() => session.setDefaultStorageBackend(isDefault ? '' : id))
  }, [id, isDefault, run, session])

  const handleRemove = useCallback(() => {
    if (!session) return
    if (!confirmRemove) {
      setConfirmRemove(true)
      return
    }
    void run(() => session.removeStorageBackend(id))
  }, [confirmRemove, id, run, session])

  return (
    <div sx={[[false, edgeSpacing.s3], sx]} data-testid="storage-backend-row">
      <div sx={stylexStyles2.rule1}>
        <div sx={stylexStyles2.rule2}>
          <div sx={stylexStyles2.rule3}>
            <h3 sx={stylexStyles2.rule4}>{name}</h3>
            {isDefault && <span sx={stylexStyles2.rule5}>New Spaces</span>}
          </div>
          <p sx={stylexStyles2.rule6}>{formatStorageLocation(backend?.s3)}</p>
        </div>
        <div sx={stylexStyles2.rule7}>
          <DashboardButton
            icon={<LuPlugZap sx={stylexStyles.rule1} />}
            onClick={() => void handleCheck()}
            disabled={busy}
          >
            Check
          </DashboardButton>
          <DashboardButton
            icon={
              isDefault ? (
                <LuStarOff sx={stylexStyles.rule1} />
              ) : (
                <LuStar sx={stylexStyles.rule1} />
              )
            }
            onClick={handleDefault}
            disabled={busy}
            title={
              isDefault
                ? "New Spaces use the account's own storage"
                : 'New Spaces store their data in this bucket'
            }
          >
            {isDefault ? 'Stop using for new' : 'Use for new'}
          </DashboardButton>
          <DashboardButton
            icon={<LuTrash2 sx={stylexStyles.rule1} />}
            variant="destructive"
            onClick={handleRemove}
            onBlur={() => setConfirmRemove(false)}
            disabled={busy || placed.length !== 0}
            title={
              placed.length !== 0
                ? 'Move or delete the Spaces stored here first'
                : undefined
            }
          >
            {confirmRemove ? 'Confirm remove' : 'Remove'}
          </DashboardButton>
        </div>
      </div>

      {check && (
        <div sx={stylexStyles2.rule8}>
          <StorageCheckResult
            check={check}
            corsRule={buildStorageCorsRule(preset, window.location.origin)}
          />
        </div>
      )}

      {placed.length === 0 ? (
        <p sx={stylexStyles2.rule9}>No Spaces yet.</p>
      ) : (
        <ul sx={stylexStyles3.rule1}>
          {placed.map((space) => (
            <li sx={nativeChildSpacing.s2} key={space.spaceId}>
              <div sx={stylexStyles2.rule10}>{space.name || space.spaceId}</div>
              <SpaceUploadStatus
                sharedObjectId={space.spaceId ?? ''}
                preset={preset}
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// edgeSpacing keeps first and last row padding at the row owner.
const edgeSpacing = stylex.create({
  s3: {
    paddingTop: { default: 'calc(var(--spacing) * 3)', ':first-child': 0 },
    paddingBottom: { default: 'calc(var(--spacing) * 3)', ':last-child': 0 },
  },
})
