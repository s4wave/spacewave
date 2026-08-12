import { useCallback } from 'react'
import { LuKeyRound, LuLockKeyhole, LuShieldCheck } from 'react-icons/lu'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { SecretHandle, SecretTypeID } from '@s4wave/sdk/secret/secret.js'
import type { SecretState } from '@s4wave/sdk/secret/secret.pb.js'
import { SharedObjectHealthStatus } from '@s4wave/core/sobject/sobject.pb.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { cn } from '@s4wave/web/style/utils.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

export { SecretTypeID }

// SecretViewer displays only the browser-safe metadata stream. Payload reads
// require a separate peer-authenticated challenge and are deliberately absent.
export function SecretViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SecretHandle,
    SecretTypeID,
  )
  const streamFactory = useCallback(
    (secret: SecretHandle, signal: AbortSignal) => secret.watchState(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: SecretState | undefined = stateResource.value ?? undefined
  const secret = state?.secret
  const grant = state?.grantStatus
  const healthStatus = state?.health?.status ?? SharedObjectHealthStatus.UNKNOWN
  const accessReady = healthStatus === SharedObjectHealthStatus.READY

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <LuKeyRound className="text-foreground-alt size-4" aria-hidden="true" />
        <span className="text-foreground truncate text-sm font-semibold tracking-tight">
          {stateResource.error ? 'Secret' : secret?.displayName || 'Secret'}
        </span>
      </div>
      <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
        {stateResource.error ? (
          <LoadingCard
            view={{
              state: 'error',
              title: 'Secret metadata unavailable',
              detail: 'The redacted Secret state stream stopped unexpectedly.',
              onRetry: stateResource.retry,
            }}
          />
        ) : stateResource.loading && !state ? (
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading secret metadata',
              detail: 'Reading the authorized, redacted Secret state.',
            }}
          />
        ) : !state ? (
          <LoadingCard
            view={{
              state: 'error',
              title: 'Secret metadata unavailable',
              detail: 'The Secret resource returned no state.',
              onRetry: stateResource.retry,
            }}
          />
        ) : (
          <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
            <InfoCard
              icon={<LuLockKeyhole className="text-brand size-4" />}
              title="Protected value"
            >
              <div className="border-foreground/8 bg-background-dark text-foreground rounded-md border px-3 py-2 font-mono text-sm tracking-[0.2em]">
                ••••••••••••
              </div>
              <p className="text-foreground-alt/60 mt-2 text-xs leading-relaxed">
                Secret values stay hidden in the object viewer. Use the feature
                that consumes this credential to verify or replace it.
              </p>
            </InfoCard>

            <InfoCard title="Metadata">
              <div className="grid gap-x-6 gap-y-3 sm:grid-cols-2">
                <Metadata
                  label="Name"
                  value={state.secret?.displayName || 'Unnamed secret'}
                />
                <Metadata
                  label="Kind"
                  value={state.secret?.kind || 'Unspecified'}
                  mono
                />
                <Metadata
                  label="Object key"
                  value={objectKey || 'Unknown'}
                  mono
                />
                {state.secret?.nestedSharedObjectId ? (
                  <CopyableField
                    label="Nested SharedObject ID"
                    value={state.secret.nestedSharedObjectId}
                  />
                ) : (
                  <Metadata
                    label="Nested SharedObject ID"
                    value="Unavailable"
                  />
                )}
                <Metadata
                  label="Created"
                  value={formatTimestamp(state.secret?.createdAt)}
                />
                <Metadata
                  label="Updated"
                  value={formatTimestamp(state.secret?.updatedAt)}
                />
              </div>
            </InfoCard>

            <InfoCard
              icon={<LuShieldCheck className="text-foreground-alt size-4" />}
              title="Access status"
            >
              {accessReady ? (
                <div className="grid gap-3 sm:grid-cols-3">
                  <Metadata
                    label="Nested SharedObject readability"
                    value={
                      grant
                        ? grant.readable
                          ? 'Readable'
                          : 'Not readable'
                        : 'Unknown'
                    }
                  />
                  <Metadata
                    label="Participant"
                    value={
                      grant
                        ? grant.participant
                          ? 'Granted'
                          : 'Not granted'
                        : 'Unknown'
                    }
                  />
                  <Metadata
                    label="Active grants"
                    value={grant ? String(grant.grantCount ?? 0) : 'Unknown'}
                  />
                </div>
              ) : (
                <Metadata
                  label="Nested SharedObject status"
                  value={formatHealthStatus(healthStatus)}
                />
              )}
              <p className="text-foreground-alt/60 mt-3 text-xs leading-relaxed">
                This status describes nested SharedObject access. It does not
                reveal or copy the protected value.
              </p>
            </InfoCard>
          </div>
        )}
      </div>
    </div>
  )
}

function Metadata({
  label,
  value,
  mono = false,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="min-w-0">
      <p className="text-foreground-alt/60 text-xs">{label}</p>
      <p
        className={cn(
          'text-foreground mt-0.5 break-words text-xs',
          mono ? 'font-mono' : 'font-medium',
        )}
      >
        {value}
      </p>
    </div>
  )
}

function formatHealthStatus(status: SharedObjectHealthStatus): string {
  switch (status) {
    case SharedObjectHealthStatus.LOADING:
      return 'Loading'
    case SharedObjectHealthStatus.DEGRADED:
      return 'Degraded'
    case SharedObjectHealthStatus.CLOSED:
      return 'Unavailable'
    default:
      return 'Unknown'
  }
}

function formatTimestamp(value: Date | undefined): string {
  if (!value) return 'Unavailable'
  return value.toLocaleString()
}
