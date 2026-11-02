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

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-success'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
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
  sharedObjectId,
  preset,
}: SpaceUploadStatusProps) {
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
    return <p className="text-foreground-alt/60 text-xs">Checking upload</p>
  }

  const pending = Number(storage.pendingBlocks ?? 0n)
  const pendingLabel = `${plural(pending, 'block')} (${formatBytes(storage.pendingBytes)}) waiting to upload`
  if (!uploadError) {
    return (
      <p className="text-foreground-alt flex items-center gap-1.5 text-xs">
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
    <div className="space-y-2">
      <p className="text-warning flex items-center gap-1.5 text-xs">
        <LuUpload sx={stylexStyles.rule2} aria-hidden="true" />
        {pendingLabel}. Uploads are failing.
      </p>
      {checkView && !checkView.ok ? (
        <StorageCheckResult
          check={checkView}
          corsRule={buildStorageCorsRule(preset, window.location.origin)}
        />
      ) : (
        <p className="text-foreground-alt/60 text-xs break-words">
          {uploadError}
        </p>
      )}
    </div>
  )
}
