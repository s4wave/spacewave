import { useCallback } from 'react'
import { LuKeyRound, LuLockKeyhole, LuShieldCheck } from 'react-icons/lu'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { SecretHandle, SecretTypeID } from '@s4wave/sdk/secret/secret.js'
import type { SecretState } from '@s4wave/sdk/secret/secret.pb.js'
import { SharedObjectHealthStatus } from '@s4wave/core/sobject/sobject.pb.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import { cn } from '@s4wave/web/style/utils.js'
import { FactList, FactRow } from '@s4wave/web/ui/FactList.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'

export { SecretTypeID }

type SecretGrant = SecretState['grantStatus']

/** grantLabel describes a boolean grant flag, or Unknown before the grant arrives. */
function grantLabel(
  grant: SecretGrant,
  flag: boolean | undefined,
  yes: string,
  no: string,
): string {
  if (!grant) return 'Unknown'
  return flag ? yes : no
}

/** ProtectedValueCard explains why the secret value is never shown. */
function ProtectedValueCard() {
  return (
    <InfoCard
      icon={<LuLockKeyhole className="text-brand size-4" />}
      title="Protected value"
    >
      <div className="border-foreground/8 bg-background-dark text-foreground tracking-brand-wide rounded-md border px-3 py-2 font-mono text-sm">
        ••••••••••••
      </div>
      <p className="text-foreground-alt/60 mt-2 text-xs leading-relaxed">
        Secret values stay hidden in the object viewer. Use the feature that
        consumes this credential to verify or replace it.
      </p>
    </InfoCard>
  )
}

/** SecretMetadataCard lists the redacted secret fields. */
function SecretMetadataCard({
  secret,
  objectKey,
}: {
  secret: SecretState['secret']
  objectKey: string
}) {
  return (
    <InfoCard title="Metadata">
      <FactList>
        <FactRow
          kind="text"
          label="Name"
          value={secret?.displayName || 'Unnamed secret'}
          control={null}
        />
        {secret?.kind ? (
          <FactRow label="Kind" value={secret.kind} control={null} />
        ) : (
          <FactRow
            kind="text"
            label="Kind"
            value="Unspecified"
            control={null}
          />
        )}
        {objectKey ? (
          <FactRow label="Object key" value={objectKey} />
        ) : (
          <FactRow
            kind="text"
            label="Object key"
            value="Unknown"
            control={null}
          />
        )}
        {secret?.nestedSharedObjectId ? (
          <FactRow
            label="Nested SharedObject ID"
            value={secret.nestedSharedObjectId}
          />
        ) : (
          <FactRow
            kind="text"
            label="Nested SharedObject ID"
            value="Unavailable"
            control={null}
          />
        )}
        <FactRow
          kind="text"
          label="Created"
          value={formatTimestamp(secret?.createdAt)}
          control={null}
        />
        <FactRow
          kind="text"
          label="Updated"
          value={formatTimestamp(secret?.updatedAt)}
          control={null}
        />
      </FactList>
    </InfoCard>
  )
}

/** SecretAccessCard reports nested SharedObject access once its health is ready. */
function SecretAccessCard({
  grant,
  healthStatus,
}: {
  grant: SecretGrant
  healthStatus: SharedObjectHealthStatus
}) {
  return (
    <InfoCard
      icon={<LuShieldCheck className="text-foreground-alt size-4" />}
      title="Access status"
    >
      {healthStatus === SharedObjectHealthStatus.READY ? (
        <div className="grid gap-3 sm:grid-cols-3">
          <Metadata
            label="Nested SharedObject readability"
            value={grantLabel(
              grant,
              grant?.readable,
              'Readable',
              'Not readable',
            )}
          />
          <Metadata
            label="Participant"
            value={grantLabel(
              grant,
              grant?.participant,
              'Granted',
              'Not granted',
            )}
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
        This status describes nested SharedObject access. It does not reveal or
        copy the protected value.
      </p>
    </InfoCard>
  )
}

/** SecretDetails shows the protected value, metadata, and access cards. */
function SecretDetails({
  state,
  objectKey,
}: {
  state: SecretState
  objectKey: string
}) {
  const healthStatus = state.health?.status ?? SharedObjectHealthStatus.UNKNOWN

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
      <ProtectedValueCard />
      <SecretMetadataCard secret={state.secret} objectKey={objectKey} />
      <SecretAccessCard grant={state.grantStatus} healthStatus={healthStatus} />
    </div>
  )
}

/** SecretBody renders the state stream as an error, loading card, or the details. */
function SecretBody({
  error,
  loading,
  state,
  retry,
  objectKey,
}: {
  error: Error | null | undefined
  loading: boolean
  state: SecretState | undefined
  retry: () => void
  objectKey: string
}) {
  if (error) {
    return (
      <LoadingCard
        view={{
          state: 'error',
          title: 'Secret metadata unavailable',
          detail: 'The redacted Secret state stream stopped unexpectedly.',
          onRetry: retry,
        }}
      />
    )
  }
  if (!state) {
    return loading ? (
      <LoadingCard
        view={{
          state: 'active',
          title: 'Loading secret metadata',
          detail: 'Reading the authorized, redacted Secret state.',
        }}
      />
    ) : (
      <LoadingCard
        view={{
          state: 'error',
          title: 'Secret metadata unavailable',
          detail: 'The Secret resource returned no state.',
          onRetry: retry,
        }}
      />
    )
  }
  return <SecretDetails state={state} objectKey={objectKey} />
}

// SecretViewer displays only the browser-safe metadata stream. Payload reads
// require a separate peer-authenticated challenge and are deliberately absent.
export function SecretViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  // Open the secret's typed handle by its key.
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
  useObjectName(state?.secret?.displayName)
  const title = stateResource.error ? 'Secret' : state?.secret?.displayName

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <ViewerBar icon={<LuKeyRound />} title={title || 'Secret'} />
      <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
        <SecretBody
          error={stateResource.error}
          loading={stateResource.loading}
          state={state}
          retry={stateResource.retry}
          objectKey={objectKey}
        />
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
