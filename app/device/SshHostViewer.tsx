import { useCallback, type ReactNode } from 'react'
import {
  LuActivity,
  LuHardDrive,
  LuServer,
  LuShield,
  LuTerminal,
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
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { buildCreateSshHostTerminalOpData } from './terminal-action.js'

export { SshHostTypeID }

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
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to open terminal',
      )
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
      <div className="min-h-0 flex-1 overflow-auto p-4">
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
          <div className="mx-auto flex w-full max-w-4xl flex-col gap-5">
            <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <HostField label="Mode" value="SSH-only" />
              <HostField
                label="Probe"
                value={formatProbeState(state.lastStatus?.state)}
              />
              <HostField label="Endpoint" value={formatEndpoint(state)} />
              <HostField
                label="User"
                value={state.endpoint?.username || 'unknown'}
              />
            </section>

            <section className="grid gap-3 lg:grid-cols-2">
              <Panel title="Status" icon={<LuActivity className="size-3.5" />}>
                <div className="text-foreground text-sm">
                  {state.lastStatus?.message || 'No probe recorded'}
                </div>
                {state.lastStatus?.error && (
                  <div className="text-destructive mt-2 text-sm">
                    {state.lastStatus.error}
                  </div>
                )}
              </Panel>
              <Panel
                title="Credentials"
                icon={<LuShield className="size-3.5" />}
              >
                <CredentialRows refs={state.credentials} />
              </Panel>
            </section>

            <Panel title="Trust" icon={<LuShield className="size-3.5" />}>
              {pins.length === 0 ? (
                <div className="text-muted-foreground text-sm">
                  No host key pins
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
            </Panel>
          </div>
        )}
      </div>
    </div>
  )
}

function HostField({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-foreground/10 bg-background-secondary rounded-md border p-3">
      <div className="text-muted-foreground text-xs font-medium uppercase">
        {label}
      </div>
      <div className="text-foreground mt-2 min-w-0 truncate text-sm font-medium">
        {value}
      </div>
    </div>
  )
}

function Panel({
  title,
  icon,
  children,
}: {
  title: string
  icon: ReactNode
  children: ReactNode
}) {
  return (
    <div className="border-foreground/10 bg-background-secondary rounded-md border p-3">
      <div className="text-muted-foreground mb-3 flex items-center gap-2 text-xs font-medium uppercase">
        {icon}
        {title}
      </div>
      {children}
    </div>
  )
}

function CredentialRows({ refs }: { refs?: SshHostCredentialRefs }) {
  const rows = [
    ['Private key', refs?.privateKeySecretObjectKey],
    ['Password', refs?.passwordSecretObjectKey],
    ['Passphrase', refs?.passphraseSecretObjectKey],
  ] as const
  return (
    <div className="divide-foreground/8 divide-y">
      {rows.map(([label, objectKey]) => (
        <div
          key={label}
          className="grid gap-1 py-2 first:pt-0 last:pb-0 sm:grid-cols-[8rem_1fr_auto]"
        >
          <div className="text-muted-foreground text-sm">{label}</div>
          <div className="text-foreground min-w-0 truncate font-mono text-xs">
            {objectKey || 'not linked'}
          </div>
          <div className="text-muted-foreground text-xs">
            {objectKey ? 'Linked' : 'Missing'}
          </div>
        </div>
      ))}
    </div>
  )
}

function TrustPinRow({ pin }: { pin: SshHostKeyPin }) {
  return (
    <div className="grid gap-1 py-2 first:pt-0 last:pb-0 sm:grid-cols-[8rem_1fr]">
      <div className="text-muted-foreground text-sm">
        {pin.algorithm || 'unknown'}
      </div>
      <div className="text-foreground min-w-0 truncate font-mono text-xs">
        {pin.sha256Fingerprint || pin.publicKey || 'unfingerprinted'}
      </div>
    </div>
  )
}

function formatEndpoint(state: SshHost): string {
  const endpoint = state.endpoint
  if (!endpoint?.host) return 'unknown'
  return `${endpoint.host}:${endpoint.port || 22}`
}

function formatProbeState(state?: SshHostProbeState): string {
  switch (state) {
    case SshHostProbeState.READY:
      return 'Ready'
    case SshHostProbeState.FAILED:
      return 'Failed'
    default:
      return 'Unknown'
  }
}
