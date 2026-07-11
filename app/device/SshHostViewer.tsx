import { useCallback } from 'react'
import {
  LuCircleAlert,
  LuCircleCheck,
  LuHardDrive,
  LuServer,
  LuShield,
  LuTerminal,
  LuCircleX,
} from 'react-icons/lu'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { SshHostHandle, SshHostTypeID } from '@s4wave/sdk/sshhost/sshhost.js'
import {
  SshHostProbeState,
  type SshHost,
  type SshHostCredentialRefs,
  type SshHostKeyPin,
} from '@s4wave/sdk/sshhost/sshhost.pb.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { cn } from '@s4wave/web/style/utils.js'

import { buildCreateSshHostTerminalOpData } from './terminal-action.js'

export { SshHostTypeID }

export interface CanonicalSshConnectionStatus {
  state: 'connecting' | 'ready' | 'failed' | 'unknown'
  label: 'Connecting' | 'Ready' | 'Failed' | 'Unknown'
  detail: string
  observedAt?: Date
}

export function projectSshConnectionStatus(
  lastStatus: SshHost['lastStatus'] | undefined,
  loading = false,
): CanonicalSshConnectionStatus {
  const error = lastStatus?.error?.trim() ?? ''
  const message = lastStatus?.message?.trim() ?? ''
  const hasTimeout = /timeout|timed out|deadline/i.test(`${error} ${message}`)
  if (loading && !lastStatus) {
    return {
      state: 'connecting',
      label: 'Connecting',
      detail: 'Checking the SSH endpoint and host-key trust.',
    }
  }
  if (error || hasTimeout || lastStatus?.state === SshHostProbeState.FAILED) {
    const detail = hasTimeout
      ? 'The last connection attempt timed out. Check the host and network, then try again.'
      : /auth|credential|permission|denied/i.test(`${error} ${message}`)
        ? 'The last connection attempt was not accepted. Check the username and credential.'
        : 'The last connection attempt failed. Check the connection details and try again.'
    return {
      state: 'failed',
      label: 'Failed',
      detail,
      observedAt: lastStatus?.observedAt,
    }
  }
  if (lastStatus?.state === SshHostProbeState.READY) {
    return {
      state: 'ready',
      label: 'Ready',
      detail: 'Connected and host key verified.',
      observedAt: lastStatus.observedAt,
    }
  }
  return {
    state: 'unknown',
    label: 'Unknown',
    detail: 'No connection check has completed yet.',
    observedAt: lastStatus?.observedAt,
  }
}

export function SshHostViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const { navigateToObjects, spaceState, spaceWorld } =
    SpaceContainerContext.useContext()
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SshHostHandle,
    SshHostTypeID,
  )
  const streamFactory = useCallback(
    (h: SshHostHandle, signal: AbortSignal) => h.watchSshHostState(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: SshHost | undefined = stateResource.value ?? undefined
  const status = projectSshConnectionStatus(
    state?.lastStatus,
    stateResource.loading,
  )
  const pins = state?.hostKeyPins ?? []
  const existingObjectKeys = spaceState.worldContents?.objects?.map(
    (obj) => obj.objectKey ?? '',
  )

  const handleOpenTerminal = async () => {
    if (!state) return
    const terminalOp = buildCreateSshHostTerminalOpData({
      host: state,
      hostObjectKey: objectKey,
      existingObjectKeys,
    })
    if (!terminalOp) return
    try {
      await spaceWorld.applyWorldOp(
        CREATE_TERMINAL_OP_ID,
        terminalOp.opData,
        '',
      )
      navigateToObjects([terminalOp.objectKey])
    } catch {
      toast.error('Terminal could not be opened. Try again.')
    }
  }

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
        <span className="text-foreground flex min-w-0 items-center gap-2 text-sm font-semibold select-none">
          <LuServer className="size-4 shrink-0" />
          {state?.label || 'SSH Host'}
        </span>
        <div className="flex shrink-0 items-center gap-2">
          {state && (
            <DashboardButton
              icon={<LuTerminal className="size-3.5" />}
              onClick={() => void handleOpenTerminal()}
            >
              Open Terminal
            </DashboardButton>
          )}
          <DashboardButton icon={<LuHardDrive className="size-3.5" />} disabled>
            Install Agent
          </DashboardButton>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
        {stateResource.loading && !state && (
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading SSH Host',
              detail: 'Reading SSH Host state.',
            }}
          />
        )}
        {state && (
          <div className="mx-auto flex w-full max-w-4xl flex-col gap-4">
            <InfoCard>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0">
                  <div className="text-foreground text-sm font-semibold">
                    {formatEndpoint(state)}
                  </div>
                  <div className="text-foreground-alt/70 mt-1 text-xs">
                    SSH-only connection
                  </div>
                </div>
                <CanonicalStatus status={status} />
              </div>
              <div className="border-foreground/8 mt-3 grid gap-2 border-t pt-3 text-xs sm:grid-cols-4">
                <HostMeta label="Mode" value="SSH-only" />
                <HostMeta
                  label="User"
                  value={state.endpoint?.username || 'Unknown'}
                />
                <HostMeta
                  label="Credential"
                  value={credentialSummary(state.credentials)}
                />
                <HostMeta
                  label="Trust"
                  value={
                    pins.length ? 'Pinned host key' : 'Ask on first connection'
                  }
                />
              </div>
              <p className="text-foreground-alt/60 mt-3 text-xs leading-relaxed">
                Open Terminal requires a desktop/native SSH connector.
              </p>
            </InfoCard>

            <InfoCard
              icon={
                <LuCircleAlert className="text-foreground-alt/60 size-3.5" />
              }
              title="Connection status"
            >
              <div className="flex items-start gap-2">
                <CanonicalStatusIcon state={status.state} />
                <div>
                  <div className="text-foreground text-sm font-medium">
                    {status.label}
                  </div>
                  <div className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
                    {status.detail}
                  </div>
                  {status.observedAt && (
                    <div className="text-foreground-alt/50 mt-1 text-xs">
                      Last checked {formatObservedAt(status.observedAt)}
                    </div>
                  )}
                </div>
              </div>
            </InfoCard>

            <InfoCard
              icon={<LuShield className="text-foreground-alt/60 size-3.5" />}
              title="Credentials"
            >
              <CredentialRows refs={state.credentials} />
            </InfoCard>

            <InfoCard
              icon={<LuShield className="text-foreground-alt/60 size-3.5" />}
              title="Host-key trust"
            >
              {pins.length === 0 ? (
                <div className="text-foreground-alt/70 text-xs">
                  Ask on first connection
                </div>
              ) : (
                <div className="divide-foreground/8 divide-y">
                  {pins.map((pin) => (
                    <TrustPinRow
                      key={`${pin.algorithm}:${pin.sha256Fingerprint || pin.publicKey}`}
                      pin={pin}
                    />
                  ))}
                </div>
              )}
            </InfoCard>

            <div className="border-foreground/8 bg-background-card/20 rounded-lg border p-3">
              <div className="text-foreground-alt/70 text-xs leading-relaxed">
                Agent installation is not available from this SSH Host until
                secure bootstrap is configured.
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function HostMeta({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <div className="text-foreground-alt/60 text-xs">{label}</div>
      <div className="text-foreground mt-0.5 truncate text-xs font-medium">
        {value}
      </div>
    </div>
  )
}

function CanonicalStatus({ status }: { status: CanonicalSshConnectionStatus }) {
  return (
    <div className="flex shrink-0 items-center gap-1.5">
      <CanonicalStatusIcon state={status.state} />
      <span className="text-foreground text-xs font-medium">
        {status.label}
      </span>
    </div>
  )
}

function CanonicalStatusIcon({
  state,
}: {
  state: CanonicalSshConnectionStatus['state']
}) {
  const Icon =
    state === 'ready'
      ? LuCircleCheck
      : state === 'failed'
        ? LuCircleX
        : LuCircleAlert
  return (
    <Icon
      className={cn(
        'size-3.5',
        state === 'ready'
          ? 'text-success'
          : state === 'failed'
            ? 'text-destructive'
            : 'text-warning',
      )}
    />
  )
}

function CredentialRows({ refs }: { refs?: SshHostCredentialRefs }) {
  const summary = credentialSummary(refs)
  return (
    <div className="space-y-2">
      <div className="text-foreground text-sm font-medium">{summary}</div>
      <details className="text-xs">
        <summary className="text-foreground-alt/70 cursor-pointer">
          Technical credential details
        </summary>
        <div className="text-foreground-alt/60 mt-2 space-y-1 font-mono text-xs">
          <div>
            Private key: {refs?.privateKeySecretObjectKey || 'not linked'}
          </div>
          <div>Password: {refs?.passwordSecretObjectKey || 'not linked'}</div>
          <div>
            Passphrase: {refs?.passphraseSecretObjectKey || 'not linked'}
          </div>
        </div>
      </details>
    </div>
  )
}

function credentialSummary(refs?: SshHostCredentialRefs): string {
  if (refs?.privateKeySecretObjectKey) return 'Private key linked'
  if (refs?.passwordSecretObjectKey) return 'Password linked'
  return 'No credential linked'
}

function TrustPinRow({ pin }: { pin: SshHostKeyPin }) {
  return (
    <div className="grid gap-1 py-2 first:pt-0 last:pb-0 sm:grid-cols-[8rem_1fr]">
      <div className="text-foreground-alt/70 text-xs">
        {pin.algorithm || 'unknown'}
      </div>
      <div className="text-foreground min-w-0 font-mono text-xs break-all">
        {pin.sha256Fingerprint || pin.publicKey || 'unfingerprinted'}
      </div>
    </div>
  )
}

function formatEndpoint(state: SshHost): string {
  const endpoint = state.endpoint
  if (!endpoint?.host) return 'Unknown endpoint'
  return `${endpoint.host}:${endpoint.port || 22}`
}

function formatObservedAt(timestamp: Date): string {
  return timestamp.toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })
}
