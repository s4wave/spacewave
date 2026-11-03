import { useCallback, useMemo } from 'react'
import { LuCircleCheck, LuUpload } from '@s4wave/web/images/icons.js'
import { isDesktop } from '@aptre/bldr'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import { formatBytes, plural } from '@s4wave/app/system/format.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'

import { StorageCheckResult } from './StorageCheckResult.js'
import {
  buildStorageCorsRule,
  describeStorageCheck,
  type StoragePreset,
} from './storage-backend.js'
import { useSpaceStorage } from './useSpaceStorage.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-warning'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    overflowWrap: 'break-word',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-success'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

interface SpaceUploadStatusProps {
  sharedObjectId: string
  // preset selects the CORS rule format shown when uploads cannot reach the bucket.
  preset: StoragePreset
}

// SpaceUploadStatus shows whether a placed Space's blocks have reached its
// bucket. When uploads fail it checks the bucket once per distinct failure
// and shows the classified cause with its fix.
export function SpaceUploadStatus({
  sx,
  sharedObjectId,
  preset,
}: SpaceUploadStatusProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const storage = useSpaceStorage(sharedObjectId).value

  // Classify each new upload failure with a bucket check.
  const backendId = storage?.storageBackendId ?? ''
  const uploadError = storage?.uploadError ?? ''
  const check = usePromise(
    useCallback(
      (signal: AbortSignal) => {
        if (!session || !backendId || !uploadError) return undefined
        return session.checkStorageBackend(
          { storageBackendId: backendId },
          signal,
        )
      },
      [session, backendId, uploadError],
    ),
  )
  const checkView = useMemo(
    () =>
      check.data ? describeStorageCheck(check.data.result, !isDesktop) : null,
    [check.data],
  )

  if (!storage) {
    return <p sx={[stylexStyles2.rule1, sx]}>Checking upload</p>
  }

  const pending = Number(storage.pendingBlocks ?? 0n)
  const pendingLabel = `${plural(pending, 'block')} (${formatBytes(storage.pendingBytes)}) waiting to upload`
  if (!uploadError) {
    return (
      <p sx={[stylexStyles2.rule2, sx]}>
        {pending === 0 ? (
          <>
            <LuCircleCheck sx={stylexStyles.rule1} aria-hidden="true" />
            All changes uploaded
          </>
        ) : (
          <>
            <LuUpload sx={stylexStyles.rule2} aria-hidden="true" />
            {pendingLabel}
          </>
        )}
      </p>
    )
  }

  return (
    <div sx={sx}>
      <p sx={[nativeChildSpacing.s2, stylexStyles2.rule3]}>
        <LuUpload sx={stylexStyles.rule2} aria-hidden="true" />
        {pendingLabel}. Uploads are failing.
      </p>
      {checkView && !checkView.ok ? (
        <StorageCheckResult
          sx={nativeChildSpacing.s2}
          check={checkView}
          corsRule={buildStorageCorsRule(preset, window.location.origin)}
        />
      ) : (
        <p sx={[nativeChildSpacing.s2, stylexStyles2.rule4]}>{uploadError}</p>
      )}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})
